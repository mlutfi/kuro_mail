package auth

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/webmail/backend/internal/models"
)

type Handler struct {
	svc    *Service
	logger *zap.Logger
}

func NewHandler(svc *Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// RegisterRoutes mendaftarkan route auth yang TIDAK membutuhkan autentikasi.
// Route yang membutuhkan auth (me, 2fa/*, sessions, logout) didaftarkan di main.go
// melalui authProtected group yang sudah memiliki AuthMiddleware.
func (h *Handler) RegisterRoutes(router fiber.Router) {
	auth := router.Group("/auth")
	auth.Post("/login", h.Login)
	auth.Post("/login/2fa", h.VerifyTwoFA)
	auth.Post("/refresh", h.RefreshToken)
}

// Login godoc
// @Summary Login dengan email dan password
// @Tags auth
// @Accept json
// @Produce json
// @Param body body models.LoginRequest true "Kredensial login"
// @Success 200 {object} models.LoginResponse
// @Router /auth/login [post]
func (h *Handler) Login(c *fiber.Ctx) error {
	var req models.LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse("invalid request body"))
	}

	if req.Email == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse("email and password are required"))
	}

	ipAddr := c.IP()
	resp, err := h.svc.Login(c.Context(), &req, ipAddr)
	if err != nil {
		status := fiber.StatusUnauthorized
		if errors.Is(err, ErrRateLimited) {
			status = fiber.StatusTooManyRequests
		}
		h.logger.Warn("Login failed",
			zap.String("email", req.Email),
			zap.String("ip", ipAddr),
			zap.Error(err),
		)
		return c.Status(status).JSON(models.ErrorResponse(err.Error()))
	}

	h.logger.Info("Login success", zap.String("email", req.Email))
	return c.JSON(models.SuccessResponse(resp))
}

// VerifyTwoFA godoc
// @Summary Verifikasi kode 2FA setelah step-1 login
// @Tags auth
// @Accept json
// @Produce json
// @Param body body models.TwoFAVerifyRequest true "2FA verification"
// @Success 200 {object} models.LoginResponse
// @Router /auth/login/2fa [post]
func (h *Handler) VerifyTwoFA(c *fiber.Ctx) error {
	var req models.TwoFAVerifyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse("invalid request body"))
	}

	resp, err := h.svc.VerifyTwoFA(c.Context(), &req, c.IP())
	if err != nil {
		status := fiber.StatusUnauthorized
		if errors.Is(err, Err2FABlocked) {
			status = fiber.StatusTooManyRequests
		}
		return c.Status(status).JSON(models.ErrorResponse(err.Error()))
	}

	return c.JSON(models.SuccessResponse(resp))
}

// RefreshToken godoc
// @Summary Refresh access token
// @Tags auth
// @Accept json
// @Produce json
// @Router /auth/refresh [post]
func (h *Handler) RefreshToken(c *fiber.Ctx) error {
	var req models.RefreshTokenRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse("invalid request body"))
	}

	tokens, err := h.svc.RefreshToken(c.Context(), req.RefreshToken)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("invalid or expired refresh token"))
	}

	return c.JSON(models.SuccessResponse(tokens))
}

// Logout godoc
// @Summary Logout dan revoke session
// @Tags auth
// @Security BearerAuth
// @Router /auth/logout [post]
func (h *Handler) Logout(c *fiber.Ctx) error {
	claims := GetClaimsFromCtx(c)
	if claims == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized"))
	}

	// sessionID dari claims
	// In production: parse sessionID from JWT claims
	if err := h.svc.Logout(c.Context(), claims.JTI, parseUUID(claims.SessionID)); err != nil {
		h.logger.Error("Logout error", zap.Error(err))
	}

	return c.JSON(models.SuccessResponse(fiber.Map{"message": "logged out successfully"}))
}

// Me godoc
// @Summary Get current user profile
// @Tags auth
// @Security BearerAuth
// @Router /auth/me [get]
func (h *Handler) Me(c *fiber.Ctx) error {
	user := GetUserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized"))
	}
	return c.JSON(models.SuccessResponse(user.ToProfile()))
}

// SetupTOTP godoc
// @Summary Mulai setup 2FA — generate secret & QR code
// @Tags auth
// @Security BearerAuth
// @Router /auth/2fa/setup [post]
func (h *Handler) SetupTOTP(c *fiber.Ctx) error {
	user := GetUserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized"))
	}

	resp, err := h.svc.SetupTOTP(c.Context(), user)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse(err.Error()))
	}

	return c.JSON(models.SuccessResponse(resp))
}

// EnableTOTP godoc
// @Summary Aktifkan 2FA setelah verifikasi kode pertama
// @Tags auth
// @Security BearerAuth
// @Router /auth/2fa/enable [post]
func (h *Handler) EnableTOTP(c *fiber.Ctx) error {
	user := GetUserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized"))
	}

	var req models.TwoFAEnableRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse("invalid request body"))
	}

	backupCodes, err := h.svc.EnableTOTP(c.Context(), user, req.Code)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse(err.Error()))
	}

	// Penting: backup codes hanya ditampilkan SEKALI — user harus simpan
	return c.JSON(models.SuccessResponse(fiber.Map{
		"message":      "2FA enabled successfully",
		"backup_codes": backupCodes,
		"warning":      "Save these backup codes now. They will not be shown again.",
	}))
}

// DisableTOTP godoc
// @Summary Nonaktifkan 2FA
// @Tags auth
// @Security BearerAuth
// @Router /auth/2fa/disable [post]
func (h *Handler) DisableTOTP(c *fiber.Ctx) error {
	user := GetUserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized"))
	}

	var req models.TwoFAEnableRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse("invalid request body"))
	}

	if err := h.svc.DisableTOTP(c.Context(), user, req.Code); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse(err.Error()))
	}

	return c.JSON(models.SuccessResponse(fiber.Map{"message": "2FA disabled"}))
}

// GetSessions godoc
// @Summary List active sessions
// @Tags auth
// @Security BearerAuth
// @Router /auth/sessions [get]
func (h *Handler) GetSessions(c *fiber.Ctx) error {
	user := GetUserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized"))
	}

	sessions, err := h.svc.sessionRepo.GetActiveSessions(c.Context(), user.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse("failed to get sessions"))
	}

	return c.JSON(models.SuccessResponse(sessions))
}

// RevokeSession godoc
// @Summary Revoke specific session (logout device tertentu)
// @Tags auth
// @Security BearerAuth
// @Router /auth/sessions/:session_id [delete]
func (h *Handler) RevokeSession(c *fiber.Ctx) error {
	user := GetUserFromCtx(c)
	if user == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse("unauthorized"))
	}

	sessionID := parseUUID(c.Params("session_id"))
	if err := h.svc.sessionRepo.RevokeByID(c.Context(), sessionID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse("failed to revoke session"))
	}

	return c.JSON(models.SuccessResponse(fiber.Map{"message": "session revoked"}))
}
