package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/skip2/go-qrcode"
	"golang.org/x/crypto/bcrypt"

	"github.com/webmail/backend/internal/cache"
	"github.com/webmail/backend/internal/config"
	"github.com/webmail/backend/internal/models"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrAccountDisabled    = errors.New("account is disabled")
	ErrTOTPRequired       = errors.New("2FA verification required")
	ErrInvalidTOTP        = errors.New("invalid 2FA code")
	ErrTOTPAlreadyEnabled = errors.New("2FA is already enabled")
	ErrTOTPNotEnabled     = errors.New("2FA is not enabled")
	ErrInvalidTempToken   = errors.New("invalid or expired verification token")
	ErrRateLimited        = errors.New("too many attempts, please try again later")
	Err2FABlocked         = errors.New("too many 2FA attempts, account temporarily locked")
	ErrBackupCodeInvalid  = errors.New("invalid backup code")
)

type Service struct {
	cfg         *config.Config
	cache       *cache.Cache
	userRepo    UserRepository
	sessionRepo SessionRepository
	jwtSvc      *JWTService
	imapAuth    IMAPAuthenticator
}

// UserRepository interface untuk akses data user
type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	UpdateTOTPSecret(ctx context.Context, userID uuid.UUID, secret string) error
	EnableTOTP(ctx context.Context, userID uuid.UUID) error
	DisableTOTP(ctx context.Context, userID uuid.UUID) error
	UpdateBackupCodes(ctx context.Context, userID uuid.UUID, codes []string) error
	UpdateLastLogin(ctx context.Context, userID uuid.UUID, ip string) error
	UpdatePassword(ctx context.Context, userID uuid.UUID, hash string) error
}

// SessionRepository interface untuk manajemen sesi
type SessionRepository interface {
	Create(ctx context.Context, session *models.Session) error
	GetByAccessJTI(ctx context.Context, jti string) (*models.Session, error)
	GetByRefreshJTI(ctx context.Context, jti string) (*models.Session, error)
	RevokeByID(ctx context.Context, sessionID uuid.UUID) error
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) error
	GetActiveSessions(ctx context.Context, userID uuid.UUID) ([]*models.Session, error)
	UpdateLastActive(ctx context.Context, jti string) error
}

// IMAPAuthenticator interface untuk testing kredensial login
type IMAPAuthenticator interface {
	Authenticate(email, password string) error
}

func NewService(cfg *config.Config, c *cache.Cache, userRepo UserRepository, sessionRepo SessionRepository, imapAuth IMAPAuthenticator) *Service {
	return &Service{
		cfg:         cfg,
		cache:       c,
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		jwtSvc:      NewJWTService(cfg),
		imapAuth:    imapAuth,
	}
}

// ─── LOGIN STEP 1 ─────────────────────────────────────────────────────────────

// Login melakukan verifikasi kredensial. Jika 2FA aktif, return temp_token.
func (s *Service) Login(ctx context.Context, req *models.LoginRequest, ipAddr string) (*models.LoginResponse, error) {
	// Check rate limit per IP (read-only; counter is incremented only on failure below)
	blocked, remaining, err := s.cache.CheckLoginRateLimit(ctx, ipAddr)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, ErrRateLimited
	}
	_ = remaining

	var user *models.User
	var isNewUser bool

	// Ambil user dari DB
	user, err = s.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		// User tidak (belum) ada di database — fallback ke IMAP
		imapErr := s.imapAuth.Authenticate(req.Email, req.Password)
		if imapErr != nil {
			// Tetap hash password untuk prevent timing attack
			_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$dummy"), []byte(req.Password))
			_ = s.cache.IncrLoginFailure(ctx, ipAddr) // increment failed attempt counter
			return nil, ErrInvalidCredentials
		}

		// Buat user baru (Auto-Provisioning)
		hashedBytes, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		hashStr := string(hashedBytes)

		// Pisahkan email untuk display name jika perlu
		parts := strings.Split(req.Email, "@")
		displayName := parts[0]

		user = &models.User{
			Email:         req.Email,
			DisplayName:   displayName,
			PasswordHash:  &hashStr,
			IsActive:      true,
			IsAdmin:       false,
			Timezone:      "Asia/Jakarta",
			Language:      "id",
			Theme:         "light",
			EmailsPerPage: 50,
			ReplyStyle:    "reply_all",
			IMAPPort:      993,
			IMAPUseTLS:    true,
			SMTPPort:      587,
		}

		if createErr := s.userRepo.Create(ctx, user); createErr != nil {
			return nil, fmt.Errorf("failed to auto-provision user: %w", createErr)
		}

		isNewUser = true
	} else {
		// User ditemukan, periksa password hash
		if !user.IsActive {
			return nil, ErrAccountDisabled
		}

		if user.PasswordHash == nil {
			return nil, ErrInvalidCredentials
		}
		if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.Password)); err != nil {
			_ = s.cache.IncrLoginFailure(ctx, ipAddr) // increment failed attempt counter
			return nil, ErrInvalidCredentials
		}
	}

	// Update last login
	_ = s.userRepo.UpdateLastLogin(ctx, user.ID, ipAddr)

	// Cek apakah 2FA aktif (hanya jika bukan user baru)
	if !isNewUser && user.TOTPEnabled {
		// Cek apakah device ini sudah trusted
		if req.DeviceID != "" {
			trusted, _ := s.cache.IsTrustedDevice(ctx, user.ID.String(), req.DeviceID)
			if trusted {
				// Skip 2FA untuk trusted device
				return s.createFullSession(ctx, user, req, ipAddr, req.Password)
			}
		}

		// 2FA diperlukan — buat temp token (simpan password untuk nanti)
		tempToken, err := s.createTempToken(ctx, user, req, ipAddr)
		if err != nil {
			return nil, err
		}

		return &models.LoginResponse{
			RequiresTwoFA: true,
			TempToken:     tempToken,
		}, nil
	}

	// Tidak ada 2FA atau user baru — langsung buat session penuh
	return s.createFullSession(ctx, user, req, ipAddr, req.Password)
}

// ─── LOGIN STEP 2: 2FA VERIFY ─────────────────────────────────────────────────

// VerifyTwoFA memverifikasi kode TOTP setelah step-1 login berhasil
func (s *Service) VerifyTwoFA(ctx context.Context, req *models.TwoFAVerifyRequest, ipAddr string) (*models.LoginResponse, error) {
	// Ambil temp token dari Redis
	tempEntry, err := s.cache.GetTempToken(ctx, req.TempToken)
	if err != nil {
		return nil, ErrInvalidTempToken
	}

	// Check 2FA backoff
	blocked, err := s.cache.Check2FABackoff(ctx, tempEntry.UserID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, Err2FABlocked
	}

	userID, err := uuid.Parse(tempEntry.UserID)
	if err != nil {
		return nil, ErrInvalidTempToken
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, ErrInvalidTempToken
	}

	// Verifikasi kode TOTP
	valid, err := s.verifyTOTPCode(ctx, user, req.Code)
	if err != nil {
		return nil, err
	}

	if !valid {
		// Increment backoff counter
		_ = s.cache.Incr2FABackoff(ctx, tempEntry.UserID)
		return nil, ErrInvalidTOTP
	}

	// Reset backoff setelah berhasil
	_ = s.cache.Reset2FABackoff(ctx, tempEntry.UserID)

	// Hapus temp token
	_ = s.cache.DeleteTempToken(ctx, req.TempToken)

	// Decrypt password from temp entry for session storage
	imapPassword := ""
	if tempEntry.EncryptedPassword != "" {
		imapPassword, _ = s.DecryptPassword(tempEntry.EncryptedPassword)
	}

	// Buat full session
	loginReq := &models.LoginRequest{
		DeviceID:   tempEntry.DeviceID,
		DeviceName: tempEntry.DeviceName,
	}
	resp, err := s.createFullSession(ctx, user, loginReq, ipAddr, imapPassword)
	if err != nil {
		return nil, err
	}

	// Jika user ingin trust device ini
	if req.TrustDevice && tempEntry.DeviceID != "" {
		_ = s.cache.SetTrustedDevice(ctx, user.ID.String(), tempEntry.DeviceID)
	}

	return resp, nil
}

// ─── 2FA SETUP ───────────────────────────────────────────────────────────────

// SetupTOTP generate secret baru dan QR code untuk setup 2FA
func (s *Service) SetupTOTP(ctx context.Context, user *models.User) (*models.TwoFASetupResponse, error) {
	if user.TOTPEnabled {
		return nil, ErrTOTPAlreadyEnabled
	}

	// Generate secret TOTP
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "KuroMail",
		AccountName: user.Email,
		Algorithm:   otp.AlgorithmSHA1,
		Digits:      otp.DigitsSix,
		Period:      30,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate TOTP key: %w", err)
	}

	// Enkripsi secret sebelum simpan ke DB
	encryptedSecret, err := s.encryptTOTPSecret(key.Secret())
	if err != nil {
		return nil, err
	}

	if err := s.userRepo.UpdateTOTPSecret(ctx, user.ID, encryptedSecret); err != nil {
		return nil, err
	}

	// Generate QR code sebagai base64 PNG
	qrPNG, err := qrcode.Encode(key.URL(), qrcode.Medium, 256)
	if err != nil {
		return nil, fmt.Errorf("failed to generate QR code: %w", err)
	}

	return &models.TwoFASetupResponse{
		Secret:     key.Secret(),
		QRCodeURL:  key.URL(),
		QRCodeData: base64.StdEncoding.EncodeToString(qrPNG),
	}, nil
}

// EnableTOTP mengaktifkan 2FA setelah user memverifikasi kode pertama
func (s *Service) EnableTOTP(ctx context.Context, user *models.User, code string) ([]string, error) {
	if user.TOTPEnabled {
		return nil, ErrTOTPAlreadyEnabled
	}
	if user.TOTPSecret == nil {
		return nil, errors.New("2FA setup not initiated, call setup first")
	}

	// Verifikasi kode
	valid, err := s.verifyTOTPCode(ctx, user, code)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, ErrInvalidTOTP
	}

	// Generate backup codes
	backupCodes, hashedCodes, err := s.generateBackupCodes()
	if err != nil {
		return nil, err
	}

	// Simpan ke DB
	if err := s.userRepo.UpdateBackupCodes(ctx, user.ID, hashedCodes); err != nil {
		return nil, err
	}
	if err := s.userRepo.EnableTOTP(ctx, user.ID); err != nil {
		return nil, err
	}

	// Revoke semua sesi lama karena security state berubah
	_ = s.cache.RevokeAllUserSessions(ctx, user.ID.String())
	_ = s.sessionRepo.RevokeAllForUser(ctx, user.ID)

	return backupCodes, nil // Return plain codes sekali saja ke user
}

// DisableTOTP menonaktifkan 2FA
func (s *Service) DisableTOTP(ctx context.Context, user *models.User, code string) error {
	if !user.TOTPEnabled {
		return ErrTOTPNotEnabled
	}

	valid, err := s.verifyTOTPCode(ctx, user, code)
	if err != nil {
		return err
	}
	if !valid {
		return ErrInvalidTOTP
	}

	if err := s.userRepo.DisableTOTP(ctx, user.ID); err != nil {
		return err
	}

	// Revoke semua sesi
	_ = s.cache.RevokeAllUserSessions(ctx, user.ID.String())
	_ = s.sessionRepo.RevokeAllForUser(ctx, user.ID)

	return nil
}

// ─── TOKEN REFRESH ───────────────────────────────────────────────────────────

// RefreshToken generate access token baru dari refresh token yang valid
func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*models.TokenPair, error) {
	claims, err := s.jwtSvc.ValidateRefreshToken(refreshToken)
	if err != nil {
		return nil, err
	}

	// Cek session di Redis
	sessionEntry, err := s.cache.GetSession(ctx, claims.JTI)
	if err != nil {
		return nil, errors.New("session expired or invalid")
	}

	// Generate access token baru
	userID, _ := uuid.Parse(sessionEntry.UserID)
	sessionID, _ := uuid.Parse(sessionEntry.SessionID)
	newAccessJTI := uuid.NewString()

	accessToken, err := s.jwtSvc.GenerateAccessToken(userID, sessionID, newAccessJTI)
	if err != nil {
		return nil, err
	}

	// Store session entry in Redis for new access JTI.
	// Use context.Background() because the caller's Fiber context (fasthttp.RequestCtx)
	// may be recycled after the handler returns, which would cancel the Redis write.
	sessionEntry.ExpiresAt = time.Now().Add(s.cfg.JWT.AccessExpiry)
	if err := s.cache.SetSession(context.Background(), newAccessJTI, sessionEntry); err != nil {
		return nil, fmt.Errorf("failed to store refreshed session: %w", err)
	}

	return &models.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken, // Refresh token tetap sama
		ExpiresIn:    int64(s.cfg.JWT.AccessExpiry.Seconds()),
	}, nil
}

// ─── LOGOUT ──────────────────────────────────────────────────────────────────

func (s *Service) Logout(ctx context.Context, accessJTI string, sessionID uuid.UUID) error {
	_ = s.cache.DeleteSession(ctx, accessJTI)
	return s.sessionRepo.RevokeByID(ctx, sessionID)
}

// ─── INTERNAL HELPERS ────────────────────────────────────────────────────────

func (s *Service) createTempToken(ctx context.Context, user *models.User, req *models.LoginRequest, ipAddr string) (string, error) {
	token := uuid.NewString() + "-" + uuid.NewString() // Extra entropy

	// Encrypt password for temp storage
	encPw, _ := s.EncryptPassword(req.Password)

	entry := &cache.TempTokenEntry{
		UserID:            user.ID.String(),
		Email:             user.Email,
		DeviceID:          req.DeviceID,
		DeviceName:        req.DeviceName,
		IPAddress:         ipAddr,
		CreatedAt:         time.Now(),
		EncryptedPassword: encPw,
	}
	if err := s.cache.SetTempToken(ctx, token, entry); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) createFullSession(ctx context.Context, user *models.User, req *models.LoginRequest, ipAddr string, imapPassword string) (*models.LoginResponse, error) {
	sessionID := uuid.New()
	accessJTI := uuid.NewString()
	refreshJTI := uuid.NewString()

	accessToken, err := s.jwtSvc.GenerateAccessToken(user.ID, sessionID, accessJTI)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.jwtSvc.GenerateRefreshToken(user.ID, sessionID, refreshJTI)
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(s.cfg.JWT.RefreshExpiry)
	if req.RememberMe {
		expiresAt = time.Now().Add(s.cfg.JWT.TrustedDeviceExpiry)
	}

	deviceName := req.DeviceName
	deviceID := req.DeviceID
	session := &models.Session{
		ID:           sessionID,
		UserID:       user.ID,
		AccessJTI:    accessJTI,
		RefreshJTI:   refreshJTI,
		DeviceName:   &deviceName,
		DeviceID:     &deviceID,
		IPAddress:    &ipAddr,
		ExpiresAt:    expiresAt,
		LastActiveAt: time.Now(),
	}

	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, err
	}

	// Encrypt IMAP password for session storage
	encryptedPw, _ := s.EncryptPassword(imapPassword)

	// Cache session di Redis untuk fast validation.
	// IMPORTANT: Use context.Background() — the caller's Fiber context (fasthttp.RequestCtx)
	// is pooled and may be recycled after the handler returns, silently cancelling Redis writes.
	// This caused /auth/me → 401 because the session was never stored in Redis.
	cacheEntry := &cache.SessionCacheEntry{
		UserID:            user.ID.String(),
		SessionID:         sessionID.String(),
		Email:             user.Email,
		IsAdmin:           user.IsAdmin,
		ExpiresAt:         expiresAt,
		EncryptedPassword: encryptedPw,
	}
	// Store access JTI first — this is what AuthMiddleware looks up on every request.
	// If this write fails, return an error so the client doesn't get tokens for a
	// session that can never be validated (would cause perpetual 401 on /auth/me).
	if err := s.cache.SetSession(context.Background(), accessJTI, cacheEntry); err != nil {
		return nil, fmt.Errorf("failed to store session in cache: %w", err)
	}
	// Refresh JTI and user-session-set are best-effort (non-critical path).
	_ = s.cache.SetSession(context.Background(), refreshJTI, cacheEntry)
	_ = s.cache.AddUserSession(context.Background(), user.ID.String(), accessJTI)

	return &models.LoginResponse{
		RequiresTwoFA: false,
		AccessToken:   accessToken,
		RefreshToken:  refreshToken,
		User:          user.ToProfile(),
	}, nil
}

func (s *Service) verifyTOTPCode(ctx context.Context, user *models.User, code string) (bool, error) {
	if user.TOTPSecret == nil {
		return false, errors.New("TOTP not configured")
	}

	// Decrypt secret
	secret, err := s.decryptTOTPSecret(*user.TOTPSecret)
	if err != nil {
		return false, fmt.Errorf("failed to decrypt TOTP secret: %w", err)
	}

	// Cek apakah ini backup code (8 karakter alphanumeric)
	if len(code) == 8 {
		return s.verifyBackupCode(ctx, user, code)
	}

	// Verifikasi TOTP dengan window ±2 periode (60 detik) untuk mentoleransi clock drift
	valid, err := totp.ValidateCustom(code, secret, time.Now(), totp.ValidateOpts{
		Period:    30,
		Skew:      2,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return valid, err
}

func (s *Service) verifyBackupCode(ctx context.Context, user *models.User, code string) (bool, error) {
	codeHash := hashBackupCode(code)
	var remainingCodes []string
	found := false

	for _, hashedCode := range user.BackupCodes {
		if hashedCode == codeHash {
			found = true
			continue // Hapus kode yang sudah digunakan
		}
		remainingCodes = append(remainingCodes, hashedCode)
	}

	if !found {
		return false, nil
	}

	// Update backup codes di DB (hapus kode yang sudah dipakai)
	if err := s.userRepo.UpdateBackupCodes(ctx, user.ID, remainingCodes); err != nil {
		return false, err
	}

	return true, nil
}

// generateBackupCodes menghasilkan 10 backup codes baru
func (s *Service) generateBackupCodes() (plain []string, hashed []string, err error) {
	for i := 0; i < 10; i++ {
		b := make([]byte, 4)
		if _, err := rand.Read(b); err != nil {
			return nil, nil, err
		}
		code := strings.ToUpper(hex.EncodeToString(b)) // 8 karakter hex
		plain = append(plain, code)
		hashed = append(hashed, hashBackupCode(code))
	}
	return plain, hashed, nil
}

func hashBackupCode(code string) string {
	h := sha256.Sum256([]byte(strings.ToUpper(code)))
	return hex.EncodeToString(h[:])
}

// ─── TOTP ENCRYPTION ─────────────────────────────────────────────────────────

func (s *Service) encryptTOTPSecret(plaintext string) (string, error) {
	key, err := hex.DecodeString(s.cfg.Crypto.TOTPEncryptionKey)
	if err != nil || len(key) != 32 {
		return "", errors.New("invalid TOTP encryption key (must be 32 bytes hex)")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (s *Service) decryptTOTPSecret(ciphertext string) (string, error) {
	key, err := hex.DecodeString(s.cfg.Crypto.TOTPEncryptionKey)
	if err != nil || len(key) != 32 {
		return "", errors.New("invalid TOTP encryption key")
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, cipherData := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// ─── IMAP PASSWORD ENCRYPTION ────────────────────────────────────────────────

// EncryptPassword encrypts the IMAP password using APP_KEY (AES-GCM).
func (s *Service) EncryptPassword(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	key, err := hex.DecodeString(s.cfg.Crypto.AppKey)
	if err != nil || len(key) != 32 {
		return "", errors.New("invalid APP_KEY (must be 32 bytes hex)")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptPassword decrypts the IMAP password using APP_KEY (AES-GCM).
func (s *Service) DecryptPassword(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	key, err := hex.DecodeString(s.cfg.Crypto.AppKey)
	if err != nil || len(key) != 32 {
		return "", errors.New("invalid APP_KEY")
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, cipherData := data[:nonceSize], data[nonceSize:]
	plain, err := gcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return "", err
	}

	return string(plain), nil
}
