package auth

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/webmail/backend/internal/models"
)

// contextKey is an unexported type for context keys to avoid collisions.
type contextKey string

const ctxKeyIMAPPw contextKey = "imap_password"

// WithIMAPPassword returns a new context with the IMAP password.
func WithIMAPPassword(ctx context.Context, password string) context.Context {
	return context.WithValue(ctx, ctxKeyIMAPPw, password)
}

// IMAPPasswordFromCtx extracts the IMAP password from a Go context.
func IMAPPasswordFromCtx(ctx context.Context) string {
	pw, _ := ctx.Value(ctxKeyIMAPPw).(string)
	return pw
}

const (
	CtxKeyUser         = "current_user"
	CtxKeyClaims       = "jwt_claims"
	CtxKeyIMAPPassword = "imap_password"
)

func GetUserFromCtx(c *fiber.Ctx) *models.User {
	user, _ := c.Locals(CtxKeyUser).(*models.User)
	return user
}

func GetClaimsFromCtx(c *fiber.Ctx) *models.JWTClaims {
	claims, _ := c.Locals(CtxKeyClaims).(*models.JWTClaims)
	return claims
}

func GetIMAPPasswordFromCtx(c *fiber.Ctx) string {
	pw, _ := c.Locals(CtxKeyIMAPPassword).(string)
	return pw
}

func SetUserInCtx(c *fiber.Ctx, user *models.User) {
	c.Locals(CtxKeyUser, user)
}

func SetClaimsInCtx(c *fiber.Ctx, claims *models.JWTClaims) {
	c.Locals(CtxKeyClaims, claims)
}

func SetIMAPPasswordInCtx(c *fiber.Ctx, password string) {
	c.Locals(CtxKeyIMAPPassword, password)
}

// ParseUUID mem-parse UUID string, return uuid.Nil jika invalid.
// EXPORTED agar bisa digunakan dari package middleware.
func ParseUUID(s string) uuid.UUID {
	id, _ := uuid.Parse(s)
	return id
}

// parseUUID adalah alias internal untuk backward-compat di dalam package auth.
func parseUUID(s string) uuid.UUID {
	return ParseUUID(s)
}
