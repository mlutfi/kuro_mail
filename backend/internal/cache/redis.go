package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/webmail/backend/internal/config"
)

// Key prefixes — semua key mengikuti konvensi namespace:entity:id
const (
	KeySession          = "session:%s"           // session:{jti}
	KeyUserSession      = "user:%s:sessions"     // user:{id}:sessions  (set of JTIs)
	KeyTempToken        = "2fa_temp:%s"          // 2fa_temp:{token}
	KeyTrustedDevice    = "trusted_device:%s:%s" // trusted_device:{user_id}:{device_id}
	KeyRateLimitLogin   = "rate_login:%s"        // rate_login:{ip}
	KeyRateLimitSend    = "rate_send:%s"         // rate_send:{user_id}
	KeyRateLimitAPI     = "rate_api:%s"          // rate_api:{ip}
	KeyInboxCache       = "inbox:%s:%s:%d"       // inbox:{user_id}:{folder}:{page}
	KeyThreadCache      = "thread:%s:%s"         // thread:{user_id}:{thread_id}
	KeyFolderStats      = "folder_stats:%s"      // folder_stats:{user_id}
	KeyUnreadCount      = "unread:%s"            // unread:{user_id}
	KeyContactSearch    = "contacts:%s:%s"       // contacts:{user_id}:{query_prefix}
	KeyAttachTempURL    = "attach_url:%s"        // attach_url:{token}
	KeyDraftAutosave    = "draft_save:%s:%s"     // draft_save:{user_id}:{draft_id}
	KeyIMAPState        = "imap_state:%s"        // imap_state:{user_id}
	KeyNotifyChannel    = "notify:%s"            // notify:{user_id} — pub/sub channel
	Key2FABackoff       = "2fa_backoff:%s"       // 2fa_backoff:{user_id}
	KeyPasswordResetOTP = "pwd_reset:%s"         // pwd_reset:{token}
)

// TTL constants
const (
	TTLSession        = 7 * 24 * time.Hour
	TTLTempToken      = 5 * time.Minute
	TTLTrustedDevice  = 30 * 24 * time.Hour
	TTLInboxCache     = 60 * time.Second
	TTLThreadCache    = 5 * time.Minute
	TTLFolderStats    = 30 * time.Second
	TTLUnreadCount    = 10 * time.Second
	TTLContactSearch  = 10 * time.Minute
	TTLAttachTempURL  = 15 * time.Minute
	TTLRateLimitLogin = 15 * time.Minute
	TTLRateLimitSend  = 1 * time.Hour
	TTL2FABackoff     = 30 * time.Minute
	TTLPasswordReset  = 10 * time.Minute
)

type Cache struct {
	client *redis.Client
	logger *zap.Logger
}

// New membuat koneksi ke Redis
func New(cfg *config.RedisConfig, logger *zap.Logger) (*Cache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	logger.Info("Redis connected", zap.String("addr", cfg.Addr))
	return &Cache{client: client, logger: logger}, nil
}

func (c *Cache) Close() error {
	return c.client.Close()
}

// ─── GENERIC HELPERS ─────────────────────────────────────────────────────────

func (c *Cache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}
	return c.client.Set(ctx, key, data, ttl).Err()
}

func (c *Cache) Get(ctx context.Context, key string, dest interface{}) error {
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		return err // caller checks redis.Nil
	}
	return json.Unmarshal(data, dest)
}

func (c *Cache) Delete(ctx context.Context, keys ...string) error {
	return c.client.Del(ctx, keys...).Err()
}

func (c *Cache) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.client.Exists(ctx, key).Result()
	return n > 0, err
}

func (c *Cache) TTL(ctx context.Context, key string) (time.Duration, error) {
	return c.client.TTL(ctx, key).Result()
}

func (c *Cache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.client.Expire(ctx, key, ttl).Err()
}

// ─── SESSION ─────────────────────────────────────────────────────────────────

type SessionCacheEntry struct {
	UserID            string    `json:"user_id"`
	SessionID         string    `json:"session_id"`
	Email             string    `json:"email"`
	IsAdmin           bool      `json:"is_admin"`
	ExpiresAt         time.Time `json:"expires_at"`
	EncryptedPassword string    `json:"encrypted_password,omitempty"` // AES-GCM encrypted IMAP password
}

func (c *Cache) SetSession(ctx context.Context, jti string, entry *SessionCacheEntry) error {
	key := fmt.Sprintf(KeySession, jti)
	return c.Set(ctx, key, entry, TTLSession)
}

func (c *Cache) GetSession(ctx context.Context, jti string) (*SessionCacheEntry, error) {
	key := fmt.Sprintf(KeySession, jti)
	var entry SessionCacheEntry
	if err := c.Get(ctx, key, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (c *Cache) DeleteSession(ctx context.Context, jti string) error {
	key := fmt.Sprintf(KeySession, jti)
	return c.Delete(ctx, key)
}

// AddUserSession menambahkan JTI ke set sesi user (untuk revoke all sessions)
func (c *Cache) AddUserSession(ctx context.Context, userID, jti string) error {
	key := fmt.Sprintf(KeyUserSession, userID)
	return c.client.SAdd(ctx, key, jti).Err()
}

// RevokeAllUserSessions menghapus semua sesi user dari Redis
func (c *Cache) RevokeAllUserSessions(ctx context.Context, userID string) error {
	key := fmt.Sprintf(KeyUserSession, userID)
	jtis, err := c.client.SMembers(ctx, key).Result()
	if err != nil {
		return err
	}

	pipe := c.client.Pipeline()
	for _, jti := range jtis {
		sessionKey := fmt.Sprintf(KeySession, jti)
		pipe.Del(ctx, sessionKey)
	}
	pipe.Del(ctx, key)
	_, err = pipe.Exec(ctx)
	return err
}

// ─── 2FA TEMP TOKEN ──────────────────────────────────────────────────────────

type TempTokenEntry struct {
	UserID            string    `json:"user_id"`
	Email             string    `json:"email"`
	DeviceID          string    `json:"device_id"`
	DeviceName        string    `json:"device_name"`
	IPAddress         string    `json:"ip_address"`
	CreatedAt         time.Time `json:"created_at"`
	EncryptedPassword string    `json:"encrypted_password,omitempty"`
}

func (c *Cache) SetTempToken(ctx context.Context, token string, entry *TempTokenEntry) error {
	key := fmt.Sprintf(KeyTempToken, token)
	return c.Set(ctx, key, entry, TTLTempToken)
}

func (c *Cache) GetTempToken(ctx context.Context, token string) (*TempTokenEntry, error) {
	key := fmt.Sprintf(KeyTempToken, token)
	var entry TempTokenEntry
	if err := c.Get(ctx, key, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (c *Cache) DeleteTempToken(ctx context.Context, token string) error {
	return c.Delete(ctx, fmt.Sprintf(KeyTempToken, token))
}

// ─── TRUSTED DEVICE ──────────────────────────────────────────────────────────

func (c *Cache) SetTrustedDevice(ctx context.Context, userID, deviceID string) error {
	key := fmt.Sprintf(KeyTrustedDevice, userID, deviceID)
	return c.client.Set(ctx, key, "1", TTLTrustedDevice).Err()
}

func (c *Cache) IsTrustedDevice(ctx context.Context, userID, deviceID string) (bool, error) {
	key := fmt.Sprintf(KeyTrustedDevice, userID, deviceID)
	return c.Exists(ctx, key)
}

func (c *Cache) RevokeTrustedDevice(ctx context.Context, userID, deviceID string) error {
	key := fmt.Sprintf(KeyTrustedDevice, userID, deviceID)
	return c.Delete(ctx, key)
}

// ─── RATE LIMITING ───────────────────────────────────────────────────────────

// IncrRateLimit increment counter dan return count saat ini
// Return (current_count, is_first_increment, error)
func (c *Cache) IncrRateLimit(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	pipe := c.client.TxPipeline()
	incrCmd := pipe.Incr(ctx, key)
	pipe.ExpireNX(ctx, key, ttl) // Set TTL hanya jika key baru (NX = if not exists)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}
	return incrCmd.Val(), nil
}

func (c *Cache) GetRateLimit(ctx context.Context, key string) (int64, error) {
	val, err := c.client.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}

func (c *Cache) ResetRateLimit(ctx context.Context, key string) error {
	return c.client.Del(ctx, key).Err()
}

// CheckLoginRateLimit — read-only check, does NOT increment the counter.
// Returns blocked=true if the IP has >= 5 failed attempts in the last 15 min.
// Call IncrLoginFailure() ONLY when a login attempt actually fails.
func (c *Cache) CheckLoginRateLimit(ctx context.Context, ip string) (blocked bool, remaining int, err error) {
	key := fmt.Sprintf(KeyRateLimitLogin, ip)
	val, err := c.client.Get(ctx, key).Int64()
	if err != nil {
		if err == redis.Nil {
			return false, 5, nil // no failures recorded yet
		}
		return false, 0, err
	}
	const maxAttempts int64 = 5
	if val >= maxAttempts {
		return true, 0, nil
	}
	return false, int(maxAttempts - val), nil
}

// IncrLoginFailure increments the failed-login counter for an IP.
// Call this ONLY when authentication fails (wrong password, unknown user, etc.).
func (c *Cache) IncrLoginFailure(ctx context.Context, ip string) error {
	key := fmt.Sprintf(KeyRateLimitLogin, ip)
	_, err := c.IncrRateLimit(ctx, key, TTLRateLimitLogin)
	return err
}

// ─── 2FA BACKOFF ─────────────────────────────────────────────────────────────

// Check2FABackoff — max 5 attempts per 30 menit
func (c *Cache) Check2FABackoff(ctx context.Context, userID string) (blocked bool, err error) {
	key := fmt.Sprintf(Key2FABackoff, userID)
	count, err := c.GetRateLimit(ctx, key)
	if err != nil {
		return false, err
	}
	return count >= 5, nil
}

// Incr2FABackoff increments the failed 2FA attempt counter
func (c *Cache) Incr2FABackoff(ctx context.Context, userID string) error {
	key := fmt.Sprintf(Key2FABackoff, userID)
	_, err := c.IncrRateLimit(ctx, key, TTL2FABackoff)
	return err
}

func (c *Cache) Reset2FABackoff(ctx context.Context, userID string) error {
	return c.Delete(ctx, fmt.Sprintf(Key2FABackoff, userID))
}

// ─── INBOX CACHE ─────────────────────────────────────────────────────────────

func (c *Cache) SetInboxCache(ctx context.Context, userID, folder string, page int, data interface{}) error {
	key := fmt.Sprintf(KeyInboxCache, userID, folder, page)
	return c.Set(ctx, key, data, TTLInboxCache)
}

func (c *Cache) GetInboxCache(ctx context.Context, userID, folder string, page int, dest interface{}) error {
	key := fmt.Sprintf(KeyInboxCache, userID, folder, page)
	return c.Get(ctx, key, dest)
}

// InvalidateInboxCache menghapus seluruh cache inbox user untuk folder tertentu
func (c *Cache) InvalidateInboxCache(ctx context.Context, userID, folder string) error {
	pattern := fmt.Sprintf("inbox:%s:%s:*", userID, folder)
	return c.deleteByPattern(ctx, pattern)
}

// ─── THREAD CACHE ────────────────────────────────────────────────────────────

func (c *Cache) SetThreadCache(ctx context.Context, userID, threadID string, data interface{}) error {
	key := fmt.Sprintf(KeyThreadCache, userID, threadID)
	return c.Set(ctx, key, data, TTLThreadCache)
}

func (c *Cache) GetThreadCache(ctx context.Context, userID, threadID string, dest interface{}) error {
	key := fmt.Sprintf(KeyThreadCache, userID, threadID)
	return c.Get(ctx, key, dest)
}

func (c *Cache) InvalidateThreadCache(ctx context.Context, userID, threadID string) error {
	return c.Delete(ctx, fmt.Sprintf(KeyThreadCache, userID, threadID))
}

func (c *Cache) InvalidateAllThreadCaches(ctx context.Context, userID string) error {
	pattern := fmt.Sprintf("thread:%s:*", userID)
	return c.deleteByPattern(ctx, pattern)
}

// ─── UNREAD COUNT ────────────────────────────────────────────────────────────

func (c *Cache) SetUnreadCount(ctx context.Context, userID string, count int) error {
	key := fmt.Sprintf(KeyUnreadCount, userID)
	return c.client.Set(ctx, key, count, TTLUnreadCount).Err()
}

func (c *Cache) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	key := fmt.Sprintf(KeyUnreadCount, userID)
	val, err := c.client.Get(ctx, key).Int()
	if err == redis.Nil {
		return -1, nil // Cache miss
	}
	return val, err
}

func (c *Cache) IncrUnreadCount(ctx context.Context, userID string, delta int) error {
	key := fmt.Sprintf(KeyUnreadCount, userID)
	return c.client.IncrBy(ctx, key, int64(delta)).Err()
}

// ─── FOLDER STATS ────────────────────────────────────────────────────────────

func (c *Cache) SetFolderStats(ctx context.Context, userID string, stats interface{}) error {
	key := fmt.Sprintf(KeyFolderStats, userID)
	return c.Set(ctx, key, stats, TTLFolderStats)
}

func (c *Cache) GetFolderStats(ctx context.Context, userID string, dest interface{}) error {
	key := fmt.Sprintf(KeyFolderStats, userID)
	return c.Get(ctx, key, dest)
}

func (c *Cache) InvalidateFolderStats(ctx context.Context, userID string) error {
	return c.Delete(ctx, fmt.Sprintf(KeyFolderStats, userID), fmt.Sprintf(KeyUnreadCount, userID))
}

// ─── ATTACHMENT TEMP URL ─────────────────────────────────────────────────────

type AttachmentTempURL struct {
	UserID      string `json:"user_id"`
	StoragePath string `json:"storage_path"`
	Filename    string `json:"filename"`
	MimeType    string `json:"mime_type"`
}

func (c *Cache) SetAttachmentTempURL(ctx context.Context, token string, data *AttachmentTempURL) error {
	key := fmt.Sprintf(KeyAttachTempURL, token)
	return c.Set(ctx, key, data, TTLAttachTempURL)
}

func (c *Cache) GetAttachmentTempURL(ctx context.Context, token string) (*AttachmentTempURL, error) {
	key := fmt.Sprintf(KeyAttachTempURL, token)
	var data AttachmentTempURL
	if err := c.Get(ctx, key, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

// ─── REAL-TIME NOTIFICATIONS (Pub/Sub) ───────────────────────────────────────

type NotifyEvent struct {
	Type    string      `json:"type"` // new_email | email_read | email_deleted
	Payload interface{} `json:"payload"`
}

func (c *Cache) PublishNotification(ctx context.Context, userID string, event *NotifyEvent) error {
	channel := fmt.Sprintf(KeyNotifyChannel, userID)
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return c.client.Publish(ctx, channel, data).Err()
}

func (c *Cache) SubscribeNotifications(ctx context.Context, userID string) *redis.PubSub {
	channel := fmt.Sprintf(KeyNotifyChannel, userID)
	return c.client.Subscribe(ctx, channel)
}

// ─── DRAFT AUTOSAVE ──────────────────────────────────────────────────────────

func (c *Cache) SetDraftAutosave(ctx context.Context, userID, draftID string, data interface{}) error {
	key := fmt.Sprintf(KeyDraftAutosave, userID, draftID)
	return c.Set(ctx, key, data, 2*time.Hour)
}

func (c *Cache) GetDraftAutosave(ctx context.Context, userID, draftID string, dest interface{}) error {
	key := fmt.Sprintf(KeyDraftAutosave, userID, draftID)
	return c.Get(ctx, key, dest)
}

// ─── PASSWORD RESET OTP ───────────────────────────────────────────────────────

func (c *Cache) SetPasswordResetOTP(ctx context.Context, token, userID string) error {
	key := fmt.Sprintf(KeyPasswordResetOTP, token)
	return c.client.Set(ctx, key, userID, TTLPasswordReset).Err()
}

func (c *Cache) GetPasswordResetOTP(ctx context.Context, token string) (string, error) {
	key := fmt.Sprintf(KeyPasswordResetOTP, token)
	return c.client.Get(ctx, key).Result()
}

func (c *Cache) DeletePasswordResetOTP(ctx context.Context, token string) error {
	return c.Delete(ctx, fmt.Sprintf(KeyPasswordResetOTP, token))
}

// ─── HEALTH CHECK ────────────────────────────────────────────────────────────

func (c *Cache) HealthCheck(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// ─── INTERNAL HELPERS ────────────────────────────────────────────────────────

func (c *Cache) deleteByPattern(ctx context.Context, pattern string) error {
	var cursor uint64
	for {
		keys, nextCursor, err := c.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}
