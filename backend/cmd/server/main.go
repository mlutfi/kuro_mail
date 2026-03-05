package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/webmail/backend/internal/auth"
	"github.com/webmail/backend/internal/cache"
	"github.com/webmail/backend/internal/config"
	"github.com/webmail/backend/internal/database"
	"github.com/webmail/backend/internal/email"
	"github.com/webmail/backend/internal/middleware"
	ws "github.com/webmail/backend/internal/websocket"
)

func main() {
	// ─── LOGGER ──────────────────────────────────────────────────────────────
	logCfg := zap.NewProductionConfig()
	logCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	logger, err := logCfg.Build()
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer logger.Sync()

	// ─── CONFIG ──────────────────────────────────────────────────────────────
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	if cfg.IsDevelopment() {
		devLogger, _ := zap.NewDevelopment()
		logger = devLogger
	}

	logger.Info("Starting WebMail Backend",
		zap.String("env", cfg.App.Env),
		zap.String("port", cfg.App.Port),
		zap.String("go_version", "1.23"),
	)

	// ─── DATABASE ────────────────────────────────────────────────────────────
	db, err := database.New(&cfg.Database, logger)
	if err != nil {
		logger.Fatal("Database connection failed", zap.Error(err))
	}
	defer db.Close()

	if err := db.RunMigrations(cfg.Database.MigrationsPath); err != nil {
		logger.Fatal("Database migration failed", zap.Error(err))
	}

	// ─── REDIS ───────────────────────────────────────────────────────────────
	redisCache, err := cache.New(&cfg.Redis, logger)
	if err != nil {
		logger.Fatal("Redis connection failed", zap.Error(err))
	}
	defer redisCache.Close()

	// ─── SERVICES ────────────────────────────────────────────────────────────
	jwtSvc := auth.NewJWTService(cfg)
	imapPool := email.NewIMAPPool(&cfg.IMAP, logger)
	emailSvc := email.NewService(imapPool, redisCache, cfg, logger)

	// Repositories
	userRepo := database.NewUserRepository(db)
	sessionRepo := database.NewSessionRepository(db)

	authSvc := auth.NewService(cfg, redisCache, userRepo, sessionRepo, imapPool)

	// ─── CONTEXT FOR GRACEFUL SHUTDOWN ────────────────────────────────────────
	// Context untuk graceful shutdown semua goroutines
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	// ─── WEBSOCKET HUB ───────────────────────────────────────────────────────
	// Hanya jalankan WebSocket jika diaktifkan dalam konfigurasi
	var wsHub *ws.Hub
	if cfg.WebSocket.Enabled {
		wsHub = ws.NewHub(redisCache, logger)

		// Jalankan WebSocket hub sebagai goroutine
		go wsHub.Run(appCtx)
	} else {
		logger.Info("WebSocket is disabled via configuration")
	}

	// ─── FIBER APP ────────────────────────────────────────────────────────────
	app := fiber.New(fiber.Config{
		AppName:      "WebMail Backend v1.0",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
		BodyLimit:    50 * 1024 * 1024, // 50MB untuk attachment
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{
				"success": false,
				"error":   err.Error(),
			})
		},
	})

	// ─── GLOBAL MIDDLEWARE ───────────────────────────────────────────────────
	app.Use(recover.New(recover.Config{
		EnableStackTrace: cfg.IsDevelopment(),
	}))
	app.Use(middleware.CORSMiddleware(cfg.App.AllowOrigins))
	app.Use(middleware.RequestLogger(logger))
	app.Use(helmet.New(helmet.Config{
		// Disable XFrameOptions untuk webmail (iframe konten email)
		XFrameOptions: "",
		// CSP ketat untuk prevent XSS pada konten email
		ContentSecurityPolicy: "default-src 'self'; img-src * data:; script-src 'self'; connect-src 'self' ws: wss: http: https:",
	}))
	app.Use(compress.New())

	// ─── ROUTES ──────────────────────────────────────────────────────────────
	api := app.Group("/api/v1")

	// Health check
	api.Get("/health", func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
		defer cancel()

		dbOk := db.HealthCheck(ctx) == nil
		redisOk := redisCache.HealthCheck(ctx) == nil

		status := fiber.StatusOK
		if !dbOk || !redisOk {
			status = fiber.StatusServiceUnavailable
		}

		wsConnected := 0
		if cfg.WebSocket.Enabled && wsHub != nil {
			wsConnected = wsHub.ConnectedUsers()
		}

		return c.Status(status).JSON(fiber.Map{
			"status":       map[bool]string{true: "ok", false: "degraded"}[dbOk && redisOk],
			"database":     map[bool]string{true: "ok", false: "error"}[dbOk],
			"redis":        map[bool]string{true: "ok", false: "error"}[redisOk],
			"ws_enabled":   cfg.WebSocket.Enabled,
			"ws_connected": wsConnected,
			"timestamp":    time.Now().UTC(),
			"version":      "1.0.0",
		})
	})

	// ─── AUTH ROUTES ─────────────────────────────────────────────────────────
	authHandler := auth.NewHandler(authSvc, logger)
	authHandler.RegisterRoutes(api)

	// ─── PROTECTED AUTH ROUTES ─────────────────────────────────────────────
	// Routes yang butuh auth: /auth/me, /auth/2fa/*, /auth/sessions, /auth/logout
	// Semua di bawah prefix /auth/ agar konsisten dengan frontend API calls.
	authProtected := api.Group("/auth", middleware.AuthMiddleware(jwtSvc, redisCache, userRepo, authSvc, logger))
	authProtected.Get("/me", authHandler.Me)
	authProtected.Post("/2fa/setup", authHandler.SetupTOTP)
	authProtected.Post("/2fa/enable", authHandler.EnableTOTP)
	authProtected.Post("/2fa/disable", authHandler.DisableTOTP)
	authProtected.Get("/sessions", authHandler.GetSessions)
	authProtected.Delete("/sessions/:session_id", authHandler.RevokeSession)
	authProtected.Post("/logout", authHandler.Logout)

	// ─── WEBSOCKET ROUTES ────────────────────────────────────────────────────
	// Register WebSocket routes BEFORE email routes so that /ws path
	// is matched with WebSocketAuthMiddleware (token via query param)
	// instead of AuthMiddleware (token via Authorization header).
	if cfg.WebSocket.Enabled && wsHub != nil {
		// WebSocket endpoint: ws://host/api/v1/ws?token=<jwt>
		wsProtected := api.Group("", middleware.WebSocketAuthMiddleware(jwtSvc, redisCache, userRepo, logger))
		wsHandler := ws.NewHandler(wsHub, logger)
		wsHandler.RegisterRoutes(wsProtected)

		// WebSocket stats (monitoring)
		api.Get("/ws/stats", wsHandler.StatsHandler)
	}

	// ─── EMAIL ROUTES (protected) ────────────────────────────────────────────
	emailProtected := api.Group("", middleware.AuthMiddleware(jwtSvc, redisCache, userRepo, authSvc, logger))
	emailHandler := email.NewHandler(emailSvc, logger)
	emailHandler.RegisterRoutes(emailProtected)

	_ = jwtSvc // suppress unused

	// 404 handler
	app.Use(func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"error":   "endpoint not found",
		})
	})

	// ─── GRACEFUL SHUTDOWN ───────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		addr := ":" + cfg.App.Port
		logger.Info("Server listening",
			zap.String("addr", addr),
			zap.String("ws_endpoint", "ws://localhost"+addr+"/api/v1/ws?token=<jwt>"),
			zap.String("sse_endpoint", "GET /api/v1/email/events"),
		)
		if err := app.Listen(addr); err != nil {
			logger.Error("Server error", zap.Error(err))
		}
	}()

	<-quit
	logger.Info("Shutting down gracefully...")

	// Cancel context dulu — stop semua goroutines (IDLE watchers, WS hub, dll)
	appCancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		logger.Error("Shutdown error", zap.Error(err))
	}

	logger.Info("Server stopped cleanly")
}
