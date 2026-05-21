package middlewares

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
)

const (
	GeneralLimitPerMinute = 100
	PublicLimitPerMinute  = 30
	AuthLimitPerMinute    = 5
	SearchLimitPerMinute  = 60
	WriteLimitPerMinute   = 30
	BulkLimitPerMinute    = 5
	ExportLimitPerMinute  = 2
	UploadLimitPerMinute  = 10
	ExecuteLimitPerMinute = 5

	AuthLimitPerHour   = 20
	ExportLimitPerHour = 10
)

type RateLimitSubject string

const (
	RateLimitSubjectUser     RateLimitSubject = "user"
	RateLimitSubjectIP       RateLimitSubject = "ip"
	RateLimitSubjectUserOrIP RateLimitSubject = "user_or_ip"
)

type RateLimitPolicy struct {
	Name              string
	Limit             int
	Window            time.Duration
	Subject           RateLimitSubject
	PublicOnly        bool
	AuthenticatedOnly bool
}

func GeneralRateLimit() fiber.Handler {
	return RateLimit(RateLimitPolicy{
		Name:              "general",
		Limit:             GeneralLimitPerMinute,
		Window:            time.Minute,
		Subject:           RateLimitSubjectUser,
		AuthenticatedOnly: true,
	})
}

func PublicRateLimit() fiber.Handler {
	return RateLimit(RateLimitPolicy{
		Name:       "public",
		Limit:      PublicLimitPerMinute,
		Window:     time.Minute,
		Subject:    RateLimitSubjectIP,
		PublicOnly: true,
	})
}

func AuthRateLimit() fiber.Handler {
	return RateLimit(
		RateLimitPolicy{Name: "auth_minute", Limit: AuthLimitPerMinute, Window: time.Minute, Subject: RateLimitSubjectIP},
		RateLimitPolicy{Name: "auth_hour", Limit: AuthLimitPerHour, Window: time.Hour, Subject: RateLimitSubjectIP},
	)
}

func SearchRateLimit() fiber.Handler {
	return RateLimit(RateLimitPolicy{Name: "search", Limit: SearchLimitPerMinute, Window: time.Minute, Subject: RateLimitSubjectUserOrIP})
}

func WriteRateLimit() fiber.Handler {
	return RateLimit(RateLimitPolicy{Name: "write", Limit: WriteLimitPerMinute, Window: time.Minute, Subject: RateLimitSubjectUserOrIP})
}

func BulkRateLimit() fiber.Handler {
	return RateLimit(RateLimitPolicy{Name: "bulk", Limit: BulkLimitPerMinute, Window: time.Minute, Subject: RateLimitSubjectUserOrIP})
}

func ExportRateLimit() fiber.Handler {
	return RateLimit(
		RateLimitPolicy{Name: "export_minute", Limit: ExportLimitPerMinute, Window: time.Minute, Subject: RateLimitSubjectUserOrIP},
		RateLimitPolicy{Name: "export_hour", Limit: ExportLimitPerHour, Window: time.Hour, Subject: RateLimitSubjectUserOrIP},
	)
}

func UploadRateLimit() fiber.Handler {
	return RateLimit(RateLimitPolicy{Name: "upload", Limit: UploadLimitPerMinute, Window: time.Minute, Subject: RateLimitSubjectUserOrIP})
}

func ExecuteRateLimit() fiber.Handler {
	return RateLimit(RateLimitPolicy{Name: "execute", Limit: ExecuteLimitPerMinute, Window: time.Minute, Subject: RateLimitSubjectUserOrIP})
}

func RateLimit(policies ...RateLimitPolicy) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if configs.RedisClient == nil {
			log.Println("rate limit skipped: Redis client is not initialized")
			return c.Next()
		}
		if !configs.RedisCircuitAllow() {
			log.Println("rate limit skipped: Redis circuit breaker is open")
			return c.Next()
		}

		for _, policy := range policies {
			if policy.Limit <= 0 || policy.Window <= 0 {
				continue
			}

			authenticated := hasRateLimitUser(c)
			if policy.PublicOnly && (authenticated || hasAuthorizationHeader(c)) {
				continue
			}
			if policy.AuthenticatedOnly && !authenticated {
				continue
			}

			identity, ok := rateLimitIdentity(c, policy.Subject)
			if !ok {
				continue
			}

			count, resetAt, err := incrementRateLimit(c, policy, identity)
			if err != nil {
				log.Printf("rate limit skipped for %s: %v", policy.Name, err)
				return c.Next()
			}

			remaining := policy.Limit - int(count)
			if remaining < 0 {
				remaining = 0
			}
			c.Set("X-RateLimit-Limit", strconv.Itoa(policy.Limit))
			c.Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			c.Set("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))

			if count > int64(policy.Limit) {
				retryAfter := int(time.Until(resetAt).Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				c.Set("Retry-After", strconv.Itoa(retryAfter))
				return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
					"error":       "Rate limit exceeded",
					"limit":       policy.Limit,
					"window":      policy.Window.String(),
					"retryAfter":  retryAfter,
					"rateLimitId": policy.Name,
				})
			}
		}

		return c.Next()
	}
}

func incrementRateLimit(c *fiber.Ctx, policy RateLimitPolicy, identity string) (int64, time.Time, error) {
	now := time.Now().UTC()
	windowSeconds := int64(policy.Window.Seconds())
	windowStart := now.Unix() / windowSeconds * windowSeconds
	resetAt := time.Unix(windowStart+windowSeconds, 0).UTC()

	key := fmt.Sprintf("rate_limit:%s:%s:%d", sanitizeRedisKeyPart(policy.Name), sanitizeRedisKeyPart(identity), windowStart)

	ctx, cancel := context.WithTimeout(c.Context(), 2*time.Second)
	defer cancel()

	pipe := configs.RedisClient.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, policy.Window+time.Minute)

	if _, err := pipe.Exec(ctx); err != nil {
		configs.RedisCircuitRecordResult(err)
		return 0, resetAt, err
	}

	configs.RedisCircuitRecordSuccess()
	return incr.Val(), resetAt, nil
}

func hasRateLimitUser(c *fiber.Ctx) bool {
	if userID := localString(c, "userID"); userID != "" {
		return true
	}
	if userID := localString(c, "tenantUserID"); userID != "" {
		return true
	}
	return false
}

func hasAuthorizationHeader(c *fiber.Ctx) bool {
	return c.Get("Authorization") != ""
}

func rateLimitIdentity(c *fiber.Ctx, subject RateLimitSubject) (string, bool) {
	switch subject {
	case RateLimitSubjectIP:
		return "ip:" + c.IP(), true
	case RateLimitSubjectUser:
		userID := rateLimitUserID(c)
		if userID == "" {
			return "", false
		}
		return "user:" + rateLimitScope(c) + ":" + userID, true
	case RateLimitSubjectUserOrIP:
		if userID := rateLimitUserID(c); userID != "" {
			return "user:" + rateLimitScope(c) + ":" + userID, true
		}
		return "ip:" + rateLimitScope(c) + ":" + c.IP(), true
	default:
		return "ip:" + c.IP(), true
	}
}

func rateLimitUserID(c *fiber.Ctx) string {
	if userID := localString(c, "userID"); userID != "" {
		return userID
	}
	return localString(c, "tenantUserID")
}

func rateLimitScope(c *fiber.Ctx) string {
	tenantID := localString(c, "tenantID")
	projectID := localString(c, "projectID")

	if tenantID == "" {
		tenantID = c.Params("tenantSlug")
	}
	if projectID == "" {
		projectID = c.Params("projectSlug")
	}

	if tenantID == "" && projectID == "" {
		return "global"
	}
	return "tenant:" + tenantID + ":project:" + projectID
}

func localString(c *fiber.Ctx, key string) string {
	value, _ := c.Locals(key).(string)
	return value
}

func sanitizeRedisKeyPart(value string) string {
	replacer := strings.NewReplacer(" ", "_", "\n", "_", "\r", "_", "\t", "_")
	return replacer.Replace(value)
}
