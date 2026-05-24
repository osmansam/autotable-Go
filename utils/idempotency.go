package utils

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/responses"
)

const (
	idempotencyHeaderName    = "Idempotency-Key"
	idempotencyProcessingTTL = 30 * time.Second
	idempotencyResultTTL     = 24 * time.Hour
	idempotencyWaitTimeout   = 2 * time.Second
	idempotencyPollInterval  = 100 * time.Millisecond
)

var ErrIdempotencyRequestMismatch = errors.New("same Idempotency-Key used with different request body")

type IdempotencyBeginStatus string

const (
	IdempotencyOwned      IdempotencyBeginStatus = "owned"
	IdempotencyCompleted  IdempotencyBeginStatus = "completed"
	IdempotencyProcessing IdempotencyBeginStatus = "processing"
)

type IdempotencyProcessingRecord struct {
	RequestHash string `json:"requestHash"`
}

type IdempotencyBeginResult struct {
	Status IdempotencyBeginStatus
	Result *IdempotencyResult
}

type IdempotencyResult struct {
	Status      int                       `json:"status"`
	RequestHash string                    `json:"requestHash"`
	Body        responses.GeneralResponse `json:"body"`
}

func BuildIdempotencyRedisKey(tenantID, projectID, userID string, c *fiber.Ctx) string {
	idempotencyKey := strings.TrimSpace(c.Get(idempotencyHeaderName))
	if idempotencyKey == "" {
		return ""
	}

	method := strings.ToUpper(c.Method())
	path := normalizedOriginalURL(c)
	if userID == "" {
		userID = "anonymous"
	}

	return fmt.Sprintf("idempotency:tenant:%s:project:%s:user:%s:%s:%s:%s", tenantID, projectID, userID, method, path, idempotencyKey)
}

func normalizedOriginalURL(c *fiber.Ctx) string {
	rawURL := c.OriginalURL()
	parsedURL, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return rawURL
	}

	query := parsedURL.Query()
	for key, values := range query {
		sort.Strings(values)
		query[key] = values
	}

	if encodedQuery := query.Encode(); encodedQuery != "" {
		return parsedURL.Path + "?" + encodedQuery
	}

	return parsedURL.Path
}

func BuildIdempotencyRequestHash(c *fiber.Ctx) string {
	sum := sha256.New()
	sum.Write([]byte(strings.ToUpper(c.Method())))
	sum.Write([]byte{0})
	sum.Write([]byte(normalizedOriginalURL(c)))
	sum.Write([]byte{0})

	contentType := strings.ToLower(c.Get("Content-Type"))
	if strings.Contains(contentType, "multipart/form-data") {
		sum.Write([]byte("multipart/form-data"))
		sum.Write([]byte{0})

		form, err := c.MultipartForm()
		if err == nil && form != nil {
			valueKeys := make([]string, 0, len(form.Value))
			for key := range form.Value {
				valueKeys = append(valueKeys, key)
			}
			sort.Strings(valueKeys)
			for _, key := range valueKeys {
				values := append([]string(nil), form.Value[key]...)
				sort.Strings(values)
				for _, value := range values {
					sum.Write([]byte("value:"))
					sum.Write([]byte(key))
					sum.Write([]byte("="))
					sum.Write([]byte(value))
					sum.Write([]byte{0})
				}
			}

			fileKeys := make([]string, 0, len(form.File))
			for key := range form.File {
				fileKeys = append(fileKeys, key)
			}
			sort.Strings(fileKeys)
			for _, key := range fileKeys {
				files := form.File[key]
				sort.Slice(files, func(i, j int) bool {
					if files[i].Filename == files[j].Filename {
						return files[i].Size < files[j].Size
					}
					return files[i].Filename < files[j].Filename
				})
				for _, file := range files {
					sum.Write([]byte("file:"))
					sum.Write([]byte(key))
					sum.Write([]byte(":"))
					sum.Write([]byte(file.Filename))
					sum.Write([]byte(":"))
					sum.Write([]byte(fmt.Sprintf("%d", file.Size)))
					sum.Write([]byte(":"))
					sum.Write([]byte(file.Header.Get("Content-Type")))
					sum.Write([]byte{0})
				}
			}
		} else {
			sum.Write(c.BodyRaw())
		}

		return hex.EncodeToString(sum.Sum(nil))
	}

	sum.Write([]byte(contentType))
	sum.Write([]byte{0})
	sum.Write(c.BodyRaw())
	return hex.EncodeToString(sum.Sum(nil))
}

func GetIdempotentResult(ctx context.Context, key string, requestHash string) (*IdempotencyResult, error) {
	if key == "" || !configs.RedisCircuitAllow() {
		return nil, nil
	}

	value, err := configs.RedisClient.Get(ctx, key+":result").Result()
	configs.RedisCircuitRecordResult(err)
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var result IdempotencyResult
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		return nil, err
	}
	if result.RequestHash != "" && result.RequestHash != requestHash {
		return nil, ErrIdempotencyRequestMismatch
	}

	return &result, nil
}

func BeginIdempotentRequest(ctx context.Context, key string, requestHash string) (IdempotencyBeginResult, error) {
	if key == "" || !configs.RedisCircuitAllow() {
		return IdempotencyBeginResult{Status: IdempotencyOwned}, nil
	}

	if result, err := GetIdempotentResult(ctx, key, requestHash); err != nil {
		return IdempotencyBeginResult{Status: IdempotencyOwned}, err
	} else if result != nil {
		return IdempotencyBeginResult{
			Status: IdempotencyCompleted,
			Result: result,
		}, nil
	}

	recordPayload, err := json.Marshal(IdempotencyProcessingRecord{
		RequestHash: requestHash,
	})
	if err != nil {
		return IdempotencyBeginResult{Status: IdempotencyOwned}, err
	}

	created, err := configs.RedisClient.SetNX(ctx, key+":lock", recordPayload, idempotencyProcessingTTL).Result()
	configs.RedisCircuitRecordResult(err)
	if err != nil {
		return IdempotencyBeginResult{Status: IdempotencyOwned}, err
	}
	if !created {
		if err := ValidateProcessingRequestHash(ctx, key, requestHash); err != nil {
			return IdempotencyBeginResult{Status: IdempotencyProcessing}, err
		}
		return IdempotencyBeginResult{Status: IdempotencyProcessing}, nil
	}

	if result, err := GetIdempotentResult(ctx, key, requestHash); err != nil {
		return IdempotencyBeginResult{Status: IdempotencyOwned}, err
	} else if result != nil {
		delErr := configs.RedisClient.Del(ctx, key+":lock").Err()
		configs.RedisCircuitRecordResult(delErr)
		return IdempotencyBeginResult{
			Status: IdempotencyCompleted,
			Result: result,
		}, nil
	}

	return IdempotencyBeginResult{Status: IdempotencyOwned}, nil
}

func ValidateProcessingRequestHash(ctx context.Context, key string, requestHash string) error {
	value, err := configs.RedisClient.Get(ctx, key+":lock").Result()
	configs.RedisCircuitRecordResult(err)
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return err
	}

	var record IdempotencyProcessingRecord
	if err := json.Unmarshal([]byte(value), &record); err != nil {
		return err
	}
	if record.RequestHash != "" && record.RequestHash != requestHash {
		return ErrIdempotencyRequestMismatch
	}

	return nil
}

func StoreIdempotentResult(ctx context.Context, key string, result IdempotencyResult) error {
	if key == "" || !configs.RedisCircuitAllow() {
		return nil
	}

	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}

	err = configs.RedisClient.Set(ctx, key+":result", payload, idempotencyResultTTL).Err()
	configs.RedisCircuitRecordResult(err)
	if err != nil {
		return err
	}

	err = configs.RedisClient.Del(ctx, key+":lock").Err()
	configs.RedisCircuitRecordResult(err)
	return err
}

func WaitForIdempotentResult(ctx context.Context, key string, requestHash string) (*IdempotencyResult, error) {
	if key == "" || !configs.RedisCircuitAllow() {
		return nil, nil
	}

	deadline := time.Now().Add(idempotencyWaitTimeout)
	for time.Now().Before(deadline) {
		value, err := configs.RedisClient.Get(ctx, key+":result").Result()
		configs.RedisCircuitRecordResult(err)
		if err == nil {
			var result IdempotencyResult
			if unmarshalErr := json.Unmarshal([]byte(value), &result); unmarshalErr != nil {
				return nil, unmarshalErr
			}
			if result.RequestHash != "" && result.RequestHash != requestHash {
				return nil, ErrIdempotencyRequestMismatch
			}
			return &result, nil
		}
		if err != redis.Nil {
			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(idempotencyPollInterval):
		}
	}

	return nil, nil
}

func SendIdempotencyRequestMismatch(c *fiber.Ctx) error {
	return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
		Status:  http.StatusBadRequest,
		Message: "Same Idempotency-Key used with different request body.",
		Data:    nil,
	})
}

func SendIdempotencyInProgress(c *fiber.Ctx) error {
	return c.Status(http.StatusConflict).JSON(responses.GeneralResponse{
		Status:  http.StatusConflict,
		Message: "Request with the same Idempotency-Key is still processing.",
		Data:    nil,
	})
}
