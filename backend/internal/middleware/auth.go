package middleware

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/webmail/backend/internal/auth"
	"github.com/webmail/backend/internal/cache"
	"github.com/webmail/backend/internal/models"
)

// AuthMiddleware memvalidasi JWT token dan load user ke context.
func AuthMiddleware(jwtSvc *auth.JWTService, c *cache.Cache, userRepo auth.UserRepository, authSvc *auth.Service, logger *zap.Logger) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		authHeader := ctx.Get("Authorization")
		if authHeader == "" {
			return ctx.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("missing authorization header"))
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			return ctx.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("invalid authorization format"))
		}

		claims, err := jwtSvc.ValidateAccessToken(parts[1])
		if err != nil {
			return ctx.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("invalid or expired token"))
		}

		// Cek session di Redis (fast path)
		sessionEntry, err := c.GetSession(ctx.Context(), claims.JTI)
		if err != nil {
			return ctx.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("session expired"))
		}

		// FIX: gunakan auth.ParseUUID (exported) bukan auth.parseUUID (unexported)
		userID := auth.ParseUUID(sessionEntry.UserID)
		user, err := userRepo.GetByID(ctx.Context(), userID)
		if err != nil || !user.IsActive {
			return ctx.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("user not found or inactive"))
		}

		auth.SetClaimsInCtx(ctx, claims)
		auth.SetUserInCtx(ctx, user)

		// Decrypt IMAP password from session and set in context
		if sessionEntry.EncryptedPassword != "" && authSvc != nil {
			if pw, err := authSvc.DecryptPassword(sessionEntry.EncryptedPassword); err == nil {
				auth.SetIMAPPasswordInCtx(ctx, pw)
				// Also set in Go context for service layer access
				ctx.SetUserContext(auth.WithIMAPPassword(ctx.UserContext(), pw))
			}
		}

		// Update TTL session secara async
		go func() {
			_ = c.Expire(ctx.Context(), "session:"+claims.JTI, cache.TTLSession)
		}()

		return ctx.Next()
	}
}

// WebSocketAuthMiddleware memvalidasi token untuk upgrade WebSocket.
// Token dikirim via query param ?token=<jwt> karena WebSocket tidak support header custom.
func WebSocketAuthMiddleware(jwtSvc *auth.JWTService, c *cache.Cache, userRepo auth.UserRepository, logger *zap.Logger) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		// WebSocket: token dari query param atau header
		tokenStr := ctx.Query("token")
		if tokenStr == "" {
			// Fallback ke Authorization header (untuk SSE)
			authHeader := ctx.Get("Authorization")
			if authHeader != "" {
				parts := strings.SplitN(authHeader, " ", 2)
				if len(parts) == 2 {
					tokenStr = parts[1]
				}
			}
		}

		if tokenStr == "" {
			return ctx.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("missing token"))
		}

		claims, err := jwtSvc.ValidateAccessToken(tokenStr)
		if err != nil {
			return ctx.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("invalid token"))
		}

		sessionEntry, err := c.GetSession(ctx.Context(), claims.JTI)
		if err != nil {
			return ctx.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("session expired"))
		}

		userID := auth.ParseUUID(sessionEntry.UserID)
		user, err := userRepo.GetByID(ctx.Context(), userID)
		if err != nil || !user.IsActive {
			return ctx.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized"))
		}

		auth.SetClaimsInCtx(ctx, claims)
		auth.SetUserInCtx(ctx, user)

		return ctx.Next()
	}
}

// AdminMiddleware memastikan user adalah admin.
func AdminMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		user := auth.GetUserFromCtx(c)
		if user == nil || !user.IsAdmin {
			return c.Status(fiber.StatusForbidden).JSON(models.ErrorResponse("admin access required"))
		}
		return c.Next()
	}
}

// RateLimitMiddleware — generic rate limiter berbasis Redis.
func RateLimitMiddleware(c *cache.Cache, keyPrefix string, maxRequests int64, logger *zap.Logger) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		ip := ctx.IP()
		key := keyPrefix + ":" + ip

		count, err := c.IncrRateLimit(ctx.Context(), key, cache.TTLRateLimitLogin)
		if err != nil {
			logger.Error("Rate limit Redis error", zap.Error(err))
			return ctx.Next() // Fail open — jangan block kalau Redis error
		}

		if count > maxRequests {
			return ctx.Status(fiber.StatusTooManyRequests).JSON(models.ErrorResponse("too many requests"))
		}

		// FIX: gunakan strconv.FormatInt, bukan string(rune(n)) yang menghasilkan karakter unicode
		ctx.Set("X-RateLimit-Limit", strconv.FormatInt(maxRequests, 10))
		ctx.Set("X-RateLimit-Remaining", strconv.FormatInt(maxRequests-count, 10))
		ctx.Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(cache.TTLRateLimitLogin).Unix(), 10))

		return ctx.Next()
	}
}

// RequestLogger middleware untuk structured logging setiap request.
func RequestLogger(logger *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		latency := time.Since(start)

		logger.Info("HTTP Request",
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.Int("status", c.Response().StatusCode()),
			zap.String("ip", c.IP()),
			zap.Duration("latency", latency),
			zap.String("user_agent", c.Get("User-Agent")),
		)

		return err
	}
}

// CORSMiddleware menangani CORS headers.
func CORSMiddleware(allowOrigins string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		origin := c.Get("Origin")

		allowed := false
		for _, o := range strings.Split(allowOrigins, ",") {
			if strings.TrimSpace(o) == origin || strings.TrimSpace(o) == "*" {
				allowed = true
				break
			}
		}

		if allowed {
			c.Set("Access-Control-Allow-Origin", origin)
		}
		c.Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		c.Set("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Device-ID")
		c.Set("Access-Control-Allow-Credentials", "true")
		c.Set("Access-Control-Max-Age", "86400")

		if c.Method() == "OPTIONS" {
			return c.SendStatus(fiber.StatusNoContent)
		}

		return c.Next()
	}
}
