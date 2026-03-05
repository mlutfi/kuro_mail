package models

import (
	"time"

	"github.com/google/uuid"
)

// ─── USER ───────────────────────────────────────────────────────────────────

type User struct {
	ID          uuid.UUID `json:"id" db:"id"`
	Email       string    `json:"email" db:"email"`
	DisplayName string    `json:"display_name" db:"display_name"`
	AvatarURL   *string   `json:"avatar_url,omitempty" db:"avatar_url"`

	PasswordHash *string `json:"-" db:"password_hash"`
	IsActive     bool    `json:"is_active" db:"is_active"`
	IsAdmin      bool    `json:"is_admin" db:"is_admin"`

	// 2FA
	TOTPSecret     *string    `json:"-" db:"totp_secret"`
	TOTPEnabled    bool       `json:"totp_enabled" db:"totp_enabled"`
	TOTPVerifiedAt *time.Time `json:"totp_verified_at,omitempty" db:"totp_verified_at"`
	BackupCodes    []string   `json:"-" db:"backup_codes"`

	// Preferences
	Timezone      string `json:"timezone" db:"timezone"`
	Language      string `json:"language" db:"language"`
	Theme         string `json:"theme" db:"theme"`
	EmailsPerPage int    `json:"emails_per_page" db:"emails_per_page"`
	Signature     string `json:"signature" db:"signature"`
	ReplyStyle    string `json:"reply_style" db:"reply_style"`

	// Server config per-user (override global)
	IMAPHost   *string `json:"imap_host,omitempty" db:"imap_host"`
	IMAPPort   int     `json:"imap_port" db:"imap_port"`
	IMAPUseTLS bool    `json:"imap_use_tls" db:"imap_use_tls"`
	SMTPHost   *string `json:"smtp_host,omitempty" db:"smtp_host"`
	SMTPPort   int     `json:"smtp_port" db:"smtp_port"`

	// Timestamps
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty" db:"last_login_at"`
}

// UserProfile adalah response publik user (tanpa data sensitif)
type UserProfile struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	AvatarURL   *string   `json:"avatar_url,omitempty"`
	TOTPEnabled bool      `json:"totp_enabled"`
	Timezone    string    `json:"timezone"`
	Theme       string    `json:"theme"`
	Signature   string    `json:"signature"`
}

func (u *User) ToProfile() *UserProfile {
	return &UserProfile{
		ID:          u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		AvatarURL:   u.AvatarURL,
		TOTPEnabled: u.TOTPEnabled,
		Timezone:    u.Timezone,
		Theme:       u.Theme,
		Signature:   u.Signature,
	}
}

// ─── SESSION ─────────────────────────────────────────────────────────────────

type Session struct {
	ID           uuid.UUID `json:"id" db:"id"`
	UserID       uuid.UUID `json:"user_id" db:"user_id"`
	AccessJTI    string    `json:"-" db:"access_jti"`
	RefreshJTI   string    `json:"-" db:"refresh_jti"`
	DeviceID     *string   `json:"device_id,omitempty" db:"device_id"`
	DeviceName   *string   `json:"device_name,omitempty" db:"device_name"`
	DeviceType   *string   `json:"device_type,omitempty" db:"device_type"`
	UserAgent    *string   `json:"-" db:"user_agent"`
	IPAddress    *string   `json:"ip_address,omitempty" db:"ip_address"`
	IsTrusted    bool      `json:"is_trusted" db:"is_trusted"`
	IsRevoked    bool      `json:"is_revoked" db:"is_revoked"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	LastActiveAt time.Time `json:"last_active_at" db:"last_active_at"`
	ExpiresAt    time.Time `json:"expires_at" db:"expires_at"`
}

// ─── AUTH ─────────────────────────────────────────────────────────────────────

type LoginRequest struct {
	Email      string `json:"email" validate:"required,email"`
	Password   string `json:"password" validate:"required,min=6"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	RememberMe bool   `json:"remember_me"`
}

type LoginResponse struct {
	RequiresTwoFA bool         `json:"requires_two_fa"`
	TempToken     string       `json:"temp_token,omitempty"`
	AccessToken   string       `json:"access_token,omitempty"`
	RefreshToken  string       `json:"refresh_token,omitempty"`
	User          *UserProfile `json:"user,omitempty"`
}

type TwoFAVerifyRequest struct {
	TempToken   string `json:"temp_token" validate:"required"`
	Code        string `json:"code" validate:"required,min=6,max=8"`
	TrustDevice bool   `json:"trust_device"`
}

type TwoFASetupResponse struct {
	Secret     string `json:"secret"`
	QRCodeURL  string `json:"qr_code_url"`
	QRCodeData string `json:"qr_code_data"` // base64 PNG
}

type TwoFAEnableRequest struct {
	Code string `json:"code" validate:"required,len=6"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"` // seconds
}

// ─── EMAIL ────────────────────────────────────────────────────────────────────

type EmailAddress struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Attachment struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	Size        int64  `json:"size"`
	MimeType    string `json:"mime_type"`
	StoragePath string `json:"-"` // Tidak dikirim ke client
	ContentID   string `json:"content_id,omitempty"`
}

// EmailMessage adalah representasi satu email
type EmailMessage struct {
	ID         string `json:"id"`
	IMAPUID    uint32 `json:"imap_uid"`
	IMAPFolder string `json:"imap_folder"`
	MessageID  string `json:"message_id"`
	ThreadID   string `json:"thread_id"`
	InReplyTo  string `json:"in_reply_to,omitempty"`
	// FIX: References field ditambahkan — dibutuhkan oleh thread engine
	References []string `json:"references,omitempty"`

	Subject string         `json:"subject"`
	From    EmailAddress   `json:"from"`
	To      []EmailAddress `json:"to"`
	Cc      []EmailAddress `json:"cc,omitempty"`
	Bcc     []EmailAddress `json:"bcc,omitempty"`
	ReplyTo *EmailAddress  `json:"reply_to,omitempty"`

	BodyHTML    string `json:"body_html,omitempty"`
	BodyText    string `json:"body_text,omitempty"`
	BodyPreview string `json:"body_preview"`

	Attachments    []Attachment `json:"attachments,omitempty"`
	HasAttachments bool         `json:"has_attachments"`

	IsRead    bool     `json:"is_read"`
	IsStarred bool     `json:"is_starred"`
	IsDraft   bool     `json:"is_draft"`
	LabelIDs  []string `json:"label_ids,omitempty"`

	SentAt     *time.Time `json:"sent_at,omitempty"`
	ReceivedAt time.Time  `json:"received_at"`
}

// Thread adalah kumpulan email yang berkaitan
type Thread struct {
	ThreadID       string          `json:"thread_id"`
	Subject        string          `json:"subject"`
	Messages       []*EmailMessage `json:"messages"`
	MessageCount   int             `json:"message_count"`
	UnreadCount    int             `json:"unread_count"`
	Participants   []EmailAddress  `json:"participants"`
	HasAttachments bool            `json:"has_attachments"`
	IsStarred      bool            `json:"is_starred"`
	LabelIDs       []string        `json:"label_ids,omitempty"`
	LatestDate     time.Time       `json:"latest_date"`
}

// ThreadListItem adalah representasi thread di inbox list (ringkas)
type ThreadListItem struct {
	ThreadID       string         `json:"thread_id"`
	Subject        string         `json:"subject"`
	MessageCount   int            `json:"message_count"`
	UnreadCount    int            `json:"unread_count"`
	Participants   []EmailAddress `json:"participants"`
	BodyPreview    string         `json:"body_preview"`
	HasAttachments bool           `json:"has_attachments"`
	IsStarred      bool           `json:"is_starred"`
	LabelIDs       []string       `json:"label_ids,omitempty"`
	LatestDate     time.Time      `json:"latest_date"`
	MessageUIDs    []uint32       `json:"message_uids"`
}

// ─── PAGINATED RESPONSE ───────────────────────────────────────────────────────

type PaginatedThreads struct {
	Data       []*ThreadListItem `json:"data"`
	Total      int64             `json:"total"`
	Page       int               `json:"page"`
	PerPage    int               `json:"per_page"`
	TotalPages int               `json:"total_pages"`
	HasNext    bool              `json:"has_next"`
}

// ─── COMPOSE ─────────────────────────────────────────────────────────────────

type ComposeAttachment struct {
	Filename string `json:"filename" validate:"required"`
	MimeType string `json:"mime_type" validate:"required"`
	Base64   string `json:"base64" validate:"required"`
}

type ComposeRequest struct {
	To            []EmailAddress      `json:"to" validate:"required,min=1"`
	Cc            []EmailAddress      `json:"cc"`
	Bcc           []EmailAddress      `json:"bcc"`
	Subject       string              `json:"subject" validate:"required"`
	BodyHTML      string              `json:"body_html"`
	BodyText      string              `json:"body_text"`
	InReplyTo     string              `json:"in_reply_to,omitempty"`
	References    string              `json:"references,omitempty"`
	Attachments   []ComposeAttachment `json:"attachments,omitempty"`
	AttachmentIDs []string            `json:"attachment_ids,omitempty"`
	SendAt        *time.Time          `json:"send_at,omitempty"` // Scheduled send
}

// ─── LABEL ────────────────────────────────────────────────────────────────────

type Label struct {
	ID         uuid.UUID `json:"id" db:"id"`
	UserID     uuid.UUID `json:"user_id" db:"user_id"`
	Name       string    `json:"name" db:"name"`
	Color      string    `json:"color" db:"color"`
	Icon       *string   `json:"icon,omitempty" db:"icon"`
	IsSystem   bool      `json:"is_system" db:"is_system"`
	IMAPFolder *string   `json:"imap_folder,omitempty" db:"imap_folder"`
	Position   int       `json:"position" db:"position"`
	IsHidden   bool      `json:"is_hidden" db:"is_hidden"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}

// ─── CONTACT ─────────────────────────────────────────────────────────────────

type Contact struct {
	ID            uuid.UUID  `json:"id" db:"id"`
	UserID        uuid.UUID  `json:"-" db:"user_id"`
	Email         string     `json:"email" db:"email"`
	DisplayName   string     `json:"display_name" db:"display_name"`
	Company       *string    `json:"company,omitempty" db:"company"`
	AvatarURL     *string    `json:"avatar_url,omitempty" db:"avatar_url"`
	EmailCount    int        `json:"email_count" db:"email_count"`
	LastEmailedAt *time.Time `json:"last_emailed_at,omitempty" db:"last_emailed_at"`
}

// ─── SEARCH ───────────────────────────────────────────────────────────────────

type SearchRequest struct {
	Query     string     `query:"q" validate:"required,min=2"`
	Folder    string     `query:"folder"`
	From      string     `query:"from"`
	HasAttach bool       `query:"has_attach"`
	DateFrom  *time.Time `query:"date_from"`
	DateTo    *time.Time `query:"date_to"`
	IsUnread  *bool      `query:"unread"`
	Page      int        `query:"page"`
	PerPage   int        `query:"per_page"`
}

// ─── FOLDER STATS ─────────────────────────────────────────────────────────────

type FolderStats struct {
	Folder      string `json:"folder"`
	TotalCount  int    `json:"total_count"`
	UnreadCount int    `json:"unread_count"`
}

// ─── DRAFT ────────────────────────────────────────────────────────────────────

type Draft struct {
	ID          uuid.UUID      `json:"id" db:"id"`
	UserID      uuid.UUID      `json:"-" db:"user_id"`
	ThreadID    *string        `json:"thread_id,omitempty" db:"thread_id"`
	To          []EmailAddress `json:"to" db:"to_addresses"`
	Cc          []EmailAddress `json:"cc,omitempty" db:"cc_addresses"`
	Bcc         []EmailAddress `json:"bcc,omitempty" db:"bcc_addresses"`
	Subject     string         `json:"subject" db:"subject"`
	BodyHTML    string         `json:"body_html" db:"body_html"`
	BodyText    string         `json:"body_text" db:"body_text"`
	Attachments []Attachment   `json:"attachments,omitempty" db:"attachments"`
	SendAt      *time.Time     `json:"send_at,omitempty" db:"send_at"`
	LastSavedAt time.Time      `json:"last_saved_at" db:"last_saved_at"`
	CreatedAt   time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at" db:"updated_at"`
}

// ─── WEBSOCKET EVENT ─────────────────────────────────────────────────────────
// Struktur event yang dikirim via WebSocket ke frontend

type WSEventType string

const (
	WSEventNewEmail     WSEventType = "new_email"
	WSEventEmailRead    WSEventType = "email_read"
	WSEventEmailDeleted WSEventType = "email_deleted"
	WSEventEmailMoved   WSEventType = "email_moved"
	WSEventUnreadCount  WSEventType = "unread_count"
	WSEventConnected    WSEventType = "connected"
	WSEventPing         WSEventType = "ping"
	WSEventPong         WSEventType = "pong"
	WSEventInboxUpdate  WSEventType = "inbox_update"
)

// WSEvent adalah struktur pesan WebSocket (kompatibel dengan format Socket.IO-like)
type WSEvent struct {
	Type      WSEventType `json:"type"`
	Payload   interface{} `json:"payload,omitempty"`
	Timestamp time.Time   `json:"ts"`
}

// WSNewEmailPayload digunakan saat ada email baru masuk
type WSNewEmailPayload struct {
	Folder      string `json:"folder"`
	UnreadCount int    `json:"unread_count"`
	From        string `json:"from"`
	Subject     string `json:"subject"`
	Preview     string `json:"preview"`
}

// ─── API RESPONSE ─────────────────────────────────────────────────────────────

type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func SuccessResponse(data interface{}) APIResponse {
	return APIResponse{Success: true, Data: data}
}

func ErrorResponse(msg string) APIResponse {
	return APIResponse{Success: false, Error: msg}
}

// ─── JWT CLAIMS ───────────────────────────────────────────────────────────────

type JWTClaims struct {
	UserID    string `json:"uid"`
	SessionID string `json:"sid"`
	JTI       string `json:"jti"`
	Type      string `json:"type"` // access | refresh | temp
}
