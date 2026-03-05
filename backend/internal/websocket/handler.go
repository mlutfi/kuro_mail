package websocket

import (
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/webmail/backend/internal/auth"
	"github.com/webmail/backend/internal/models"
)

// Handler mengelola HTTP upgrade ke WebSocket.
type Handler struct {
	hub    *Hub
	logger *zap.Logger
}

// NewHandler membuat Handler WebSocket baru.
func NewHandler(hub *Hub, logger *zap.Logger) *Handler {
	return &Handler{hub: hub, logger: logger}
}

// RegisterRoutes mendaftarkan route WebSocket.
// Route ini harus dipasang SETELAH AuthMiddleware atau WebSocketAuthMiddleware.
//
// Cara penggunaan dari frontend (JavaScript):
//
//	const token = "your-jwt-access-token";
//	const ws = new WebSocket(`ws://localhost:8080/api/v1/ws?token=${token}`);
//
//	ws.onmessage = (event) => {
//	  const data = JSON.parse(event.data);
//	  // data.type: "new_email" | "email_read" | "unread_count" | ...
//	  // data.payload: { folder, unread_count, from, subject, ... }
//	  // data.ts: timestamp
//	  handleWebSocketEvent(data);
//	};
//
// Format event yang diterima sama dengan SSE (/api/v1/email/events),
// sehingga frontend bisa menggunakan salah satu atau keduanya.
func (h *Handler) RegisterRoutes(router fiber.Router) {
	// Upgrade check middleware (wajib sebelum websocket.New)
	router.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	// WebSocket endpoint
	router.Get("/ws", websocket.New(func(c *websocket.Conn) {
		// User sudah diset oleh middleware WebSocketAuthMiddleware
		user, ok := c.Locals("current_user").(*models.User)
		if !ok || user == nil {
			h.logger.Warn("WebSocket connection without valid user")
			return
		}

		h.logger.Info("WebSocket upgraded",
			zap.String("user", user.Email),
			zap.String("ip", c.RemoteAddr().String()),
		)

		// HandleConn blocking hingga koneksi ditutup
		h.hub.HandleConn(c, user)
	}, websocket.Config{
		// Timeout handshake
		HandshakeTimeout: 5e9, // 5 detik dalam nanoseconds
		// Ukuran buffer
		ReadBufferSize:  1024,
		WriteBufferSize: 4096,
		// Subprotocol (kosong = tidak pakai subprotocol)
		Subprotocols: []string{},
		// Izinkan cross-origin requests
		Origins: []string{"*"},
	}))
}

// StatsHandler mengembalikan statistik koneksi WebSocket (untuk monitoring).
func (h *Handler) StatsHandler(c *fiber.Ctx) error {
	_ = auth.GetUserFromCtx(c) // Pastikan user login (meski tidak dipakai di sini)
	return c.JSON(models.SuccessResponse(fiber.Map{
		"connected_users": h.hub.ConnectedUsers(),
	}))
}
