// Package websocket menyediakan WebSocket hub untuk real-time notifications.
//
// Arsitektur:
//   Browser ←─ WebSocket ─→ Hub ←─ Redis Pub/Sub ─→ IMAP IDLE Worker
//
// Hub mengelola semua koneksi WebSocket aktif dan meneruskan event dari
// Redis Pub/Sub ke client yang tepat. Ini memungkinkan notifikasi real-time
// seperti email baru, badge count update, dan status email.
//
// Kompatibilitas Socket.IO:
// Package ini menggunakan plain WebSocket dengan format pesan JSON yang
// mirip dengan protokol Socket.IO. Frontend dapat menggunakan:
// - Plain WebSocket API browser (direkomendasikan)
// - Atau library seperti reconnecting-websocket untuk auto-reconnect
//
// Format event (kompatibel dengan Socket.IO-like client):
//   { "type": "new_email", "payload": {...}, "ts": "2026-..." }
package websocket

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
	"go.uber.org/zap"

	"github.com/webmail/backend/internal/cache"
	"github.com/webmail/backend/internal/models"
)

// Conn adalah satu koneksi WebSocket dari satu browser tab.
type Conn struct {
	ws     *websocket.Conn
	userID string
	send   chan []byte
	hub    *Hub
}

// Hub mengelola semua koneksi WebSocket aktif.
type Hub struct {
	// userID → set of *Conn
	connections map[string]map[*Conn]bool
	mu          sync.RWMutex

	register   chan *Conn
	unregister chan *Conn

	cache  *cache.Cache
	logger *zap.Logger

	// cancelFuncs untuk stop Redis subscriber per user
	subscribers map[string]context.CancelFunc
	subMu       sync.Mutex
}

// NewHub membuat Hub baru.
func NewHub(c *cache.Cache, logger *zap.Logger) *Hub {
	return &Hub{
		connections: make(map[string]map[*Conn]bool),
		register:    make(chan *Conn, 64),
		unregister:  make(chan *Conn, 64),
		cache:       c,
		logger:      logger,
		subscribers: make(map[string]context.CancelFunc),
	}
}

// Run menjalankan event loop Hub — harus dipanggil sebagai goroutine.
func (h *Hub) Run(ctx context.Context) {
	h.logger.Info("WebSocket Hub started")

	for {
		select {
		case <-ctx.Done():
			h.logger.Info("WebSocket Hub shutting down")
			return

		case conn := <-h.register:
			h.addConn(ctx, conn)

		case conn := <-h.unregister:
			h.removeConn(conn)
		}
	}
}

func (h *Hub) addConn(ctx context.Context, conn *Conn) {
	h.mu.Lock()
	if _, ok := h.connections[conn.userID]; !ok {
		h.connections[conn.userID] = make(map[*Conn]bool)
	}
	h.connections[conn.userID][conn] = true
	connCount := len(h.connections[conn.userID])
	h.mu.Unlock()

	h.logger.Info("WebSocket client connected",
		zap.String("user_id", conn.userID),
		zap.Int("total_conns", connCount),
	)

	// Mulai Redis subscriber untuk user ini jika belum ada
	h.subMu.Lock()
	if _, exists := h.subscribers[conn.userID]; !exists {
		subCtx, cancel := context.WithCancel(ctx)
		h.subscribers[conn.userID] = cancel
		go h.subscribeUser(subCtx, conn.userID)
	}
	h.subMu.Unlock()

	// Kirim event "connected" ke client baru
	h.sendToConn(conn, &models.WSEvent{
		Type:      models.WSEventConnected,
		Payload:   map[string]string{"user_id": conn.userID},
		Timestamp: time.Now(),
	})
}

func (h *Hub) removeConn(conn *Conn) {
	h.mu.Lock()
	userConns, ok := h.connections[conn.userID]
	if ok {
		delete(userConns, conn)
		if len(userConns) == 0 {
			delete(h.connections, conn.userID)
		}
	}
	remaining := 0
	if ok {
		remaining = len(h.connections[conn.userID])
	}
	h.mu.Unlock()

	close(conn.send)

	h.logger.Info("WebSocket client disconnected",
		zap.String("user_id", conn.userID),
		zap.Int("remaining_conns", remaining),
	)

	// Stop Redis subscriber jika tidak ada koneksi aktif untuk user ini
	if remaining == 0 {
		h.subMu.Lock()
		if cancel, exists := h.subscribers[conn.userID]; exists {
			cancel()
			delete(h.subscribers, conn.userID)
		}
		h.subMu.Unlock()
	}
}

// subscribeUser subscribe ke Redis Pub/Sub dan broadcast event ke semua
// WebSocket connections user.
func (h *Hub) subscribeUser(ctx context.Context, userID string) {
	pubsub := h.cache.SubscribeNotifications(ctx, userID)
	defer pubsub.Close()

	h.logger.Debug("Redis subscriber started", zap.String("user_id", userID))

	for {
		select {
		case <-ctx.Done():
			h.logger.Debug("Redis subscriber stopped", zap.String("user_id", userID))
			return

		case msg, ok := <-pubsub.Channel():
			if !ok {
				return
			}

			// Parse event dari Redis
			var event cache.NotifyEvent
			if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil {
				h.logger.Warn("Failed to parse Redis event", zap.Error(err))
				continue
			}

			// Convert ke WSEvent dan broadcast
			wsEvent := &models.WSEvent{
				Type:      models.WSEventType(event.Type),
				Payload:   event.Payload,
				Timestamp: time.Now(),
			}

			h.BroadcastToUser(userID, wsEvent)
		}
	}
}

// BroadcastToUser mengirim event ke semua WebSocket connections user.
func (h *Hub) BroadcastToUser(userID string, event *models.WSEvent) {
	h.mu.RLock()
	conns, ok := h.connections[userID]
	if !ok {
		h.mu.RUnlock()
		return
	}
	// Buat snapshot untuk menghindari hold lock saat write
	snapshot := make([]*Conn, 0, len(conns))
	for conn := range conns {
		snapshot = append(snapshot, conn)
	}
	h.mu.RUnlock()

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	for _, conn := range snapshot {
		select {
		case conn.send <- data:
		default:
			// Channel penuh — client lambat, drop event ini
			h.logger.Warn("WebSocket send buffer full, dropping event",
				zap.String("user_id", userID))
		}
	}
}

// sendToConn mengirim event ke satu koneksi spesifik.
func (h *Hub) sendToConn(conn *Conn, event *models.WSEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	select {
	case conn.send <- data:
	default:
	}
}

// ConnectedUsers mengembalikan jumlah user yang terkoneksi WebSocket.
func (h *Hub) ConnectedUsers() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.connections)
}

// HandleConn memproses satu WebSocket connection (dipanggil dari handler).
func (h *Hub) HandleConn(ws *websocket.Conn, user *models.User) {
	conn := &Conn{
		ws:     ws,
		userID: user.ID.String(),
		send:   make(chan []byte, 64),
		hub:    h,
	}

	h.register <- conn

	// Goroutine untuk write ke WebSocket
	go conn.writePump()

	// Read pump di goroutine ini (blocking)
	conn.readPump(h)
}

// ─── CONN PUMPS ──────────────────────────────────────────────────────────────

// readPump membaca pesan dari client (untuk ping/pong dan instruksi client).
func (c *Conn) readPump(h *Hub) {
	defer func() {
		h.unregister <- c
		c.ws.Close()
	}()

	c.ws.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.ws.SetPongHandler(func(string) error {
		c.ws.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.ws.ReadMessage()
		if err != nil {
			// Connection ditutup — normal exit
			break
		}

		// Handle pesan dari client
		var incoming models.WSEvent
		if err := json.Unmarshal(message, &incoming); err != nil {
			continue
		}

		switch incoming.Type {
		case models.WSEventPing:
			h.sendToConn(c, &models.WSEvent{
				Type:      models.WSEventPong,
				Timestamp: time.Now(),
			})
		// Client bisa mengirim event lain di sini (misal: mark_read notification)
		default:
			h.logger.Debug("WS incoming event",
				zap.String("type", string(incoming.Type)),
				zap.String("user_id", c.userID),
			)
		}
	}
}

// writePump mengirim pesan ke client WebSocket.
func (c *Conn) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.ws.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// Hub menutup channel — koneksi berakhir
				c.ws.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.ws.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			// Kirim ping setiap 30 detik untuk keep-alive
			c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
