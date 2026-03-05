module github.com/webmail/backend

go 1.24.0

require (
	// ─── IMAP (Stalwart) ───────────────────────────────────────────────────
	github.com/emersion/go-imap/v2 v2.0.0-beta.5
	github.com/gofiber/contrib/websocket v1.3.2
	// ─── Web Framework ─────────────────────────────────────────────────────
	github.com/gofiber/fiber/v2 v2.52.6

	// ─── JWT ───────────────────────────────────────────────────────────────
	github.com/golang-jwt/jwt/v5 v5.2.2
	github.com/golang-migrate/migrate/v4 v4.18.2

	// ─── Utilities ─────────────────────────────────────────────────────────
	github.com/google/uuid v1.6.0

	// ─── PostgreSQL ────────────────────────────────────────────────────────
	github.com/jackc/pgx/v5 v5.8.0
	github.com/joho/godotenv v1.5.1

	// ─── 2FA ───────────────────────────────────────────────────────────────
	github.com/pquerna/otp v1.4.0

	// ─── Redis 7.x (go-redis v9) ───────────────────────────────────────────
	github.com/redis/go-redis/v9 v9.7.1
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e

	// ─── Logging & Scheduling ──────────────────────────────────────────────
	go.uber.org/zap v1.27.0
	golang.org/x/crypto v0.37.0
)

require (
	github.com/andybalholm/brotli v1.1.0 // indirect
	github.com/boombuler/barcode v1.0.2 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/emersion/go-message v0.18.2 // indirect
	github.com/emersion/go-sasl v0.0.0-20241020182733-b788ff22d5a6 // indirect
	github.com/fasthttp/websocket v1.5.8 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/klauspost/compress v1.17.9 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/savsgio/gotils v0.0.0-20240303185622-093b76447511 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.52.0 // indirect
	github.com/valyala/tcplisten v1.0.0 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	golang.org/x/net v0.34.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/sys v0.32.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)
