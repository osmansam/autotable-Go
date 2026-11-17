package ozmishowratelimit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"regexp"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
)

var identityPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Handler struct {
	config            Config
	client            redis.Cmdable
	current, previous [32]byte
	slots             chan struct{}
}

func NewHandler(config Config, client redis.Cmdable) (*Handler, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &Handler{config: config, client: client,
		current: sha256.Sum256([]byte("Bearer " + config.Secret)), previous: sha256.Sum256([]byte("Bearer " + config.PreviousSecret)),
		slots: make(chan struct{}, config.MaxConcurrent)}, nil
}

func (h *Handler) Handle(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	if c.Path() != Path {
		return failure(c, 404, "Not found")
	}
	if c.Method() != fiber.MethodPost {
		c.Set("Allow", fiber.MethodPost)
		return failure(c, 405, "Method not allowed")
	}
	presented := sha256.Sum256([]byte(c.Get("Authorization")))
	current := subtle.ConstantTimeCompare(presented[:], h.current[:])
	previous := subtle.ConstantTimeCompare(presented[:], h.previous[:])
	if current != 1 && !(h.config.PreviousSecret != "" && previous == 1) {
		return failure(c, 401, "Invalid service credentials")
	}
	if encoding := c.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return failure(c, 415, "Encoded request bodies are not supported")
	}
	if len(c.BodyRaw()) > 1024 || c.Request().Header.ContentLength() > 1024 {
		return failure(c, 413, "Request body is too large")
	}
	mediaType, _, err := mime.ParseMediaType(c.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return failure(c, 415, "JSON content type is required")
	}
	policyName, identity, err := decodeRequest(c.BodyRaw())
	if err != nil || !identityPattern.MatchString(identity) {
		return failure(c, 400, "Invalid rate limit request")
	}
	policy, ok := h.config.Policies[policyName]
	if !ok {
		return failure(c, 400, "Unknown rate limit policy")
	}
	if h.client == nil {
		return failure(c, 503, "Rate limit service unavailable")
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		return failure(c, 503, "Rate limit service busy")
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), time.Second)
	defer cancel()
	key := "ozmishow:rl:v1:" + h.config.Namespace + ":" + policyName + ":" + identity
	result, err := consume(ctx, h.client, key, policy)
	if err != nil {
		return failure(c, 503, "Rate limit service unavailable")
	}
	if !result.Allowed {
		c.Set("Retry-After", strconv.FormatInt(result.RetryAfterSeconds, 10))
		c.Status(429)
	}
	return c.JSON(result)
}

func failure(c *fiber.Ctx, status int, message string) error {
	c.Set("Cache-Control", "no-store")
	if status == 503 {
		c.Set("Retry-After", "30")
	}
	return c.Status(status).JSON(fiber.Map{"error": message})
}

// Token decoding rejects duplicate fields as well as unknown or trailing JSON.
func decodeRequest(body []byte) (string, string, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return "", "", errors.New("expected object")
	}
	fields := make(map[string]string, 2)
	for decoder.More() {
		token, err = decoder.Token()
		if err != nil {
			return "", "", err
		}
		name, ok := token.(string)
		if !ok || (name != "policy" && name != "identity") {
			return "", "", errors.New("unexpected field")
		}
		if _, exists := fields[name]; exists {
			return "", "", errors.New("duplicate field")
		}
		var value string
		if err = decoder.Decode(&value); err != nil {
			return "", "", err
		}
		fields[name] = value
	}
	if _, err = decoder.Token(); err != nil {
		return "", "", err
	}
	var extra interface{}
	if err = decoder.Decode(&extra); err != io.EOF || len(fields) != 2 {
		return "", "", errors.New("invalid JSON")
	}
	return fields["policy"], fields["identity"], nil
}
