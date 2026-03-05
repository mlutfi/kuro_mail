package database

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/webmail/backend/internal/config"
)

type DB struct {
	Pool   *pgxpool.Pool
	logger *zap.Logger
}

// New membuat koneksi pool ke PostgreSQL
func New(cfg *config.DatabaseConfig, logger *zap.Logger) (*DB, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("invalid database DSN: %w", err)
	}

	poolConfig.MaxConns = int32(cfg.MaxOpenConns)
	poolConfig.MinConns = int32(cfg.MaxIdleConns)
	poolConfig.MaxConnLifetime = cfg.ConnMaxLifetime
	poolConfig.MaxConnIdleTime = 10 * time.Minute
	poolConfig.HealthCheckPeriod = 30 * time.Second

	// Hindari error prepared statement "already exists" dengan PgBouncer atau pooling environment
	poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Ping untuk verifikasi koneksi
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("Database connected", zap.String("dsn_host", poolConfig.ConnConfig.Host))

	return &DB{Pool: pool, logger: logger}, nil
}

// RunMigrations menjalankan semua pending migrations
func (db *DB) RunMigrations(migrationsPath string) error {
	db.logger.Info("Running database migrations", zap.String("path", migrationsPath))

	m, err := migrate.New(migrationsPath, db.Pool.Config().ConnConfig.ConnString())
	if err != nil {
		return fmt.Errorf("failed to initialize migrations: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration failed: %w", err)
	}

	version, dirty, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	db.logger.Info("Migrations complete",
		zap.Uint("version", version),
		zap.Bool("dirty", dirty),
	)
	return nil
}

// Close menutup semua koneksi pool
func (db *DB) Close() {
	db.Pool.Close()
	db.logger.Info("Database connections closed")
}

// HealthCheck memverifikasi koneksi masih aktif
func (db *DB) HealthCheck(ctx context.Context) error {
	return db.Pool.Ping(ctx)
}

// Transaction helper untuk menjalankan fungsi dalam transaction
func (db *DB) WithTransaction(ctx context.Context, fn func(ctx context.Context, tx pgx.Tx) error) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	if err := fn(ctx, tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			db.logger.Error("Transaction rollback failed",
				zap.Error(rbErr),
				zap.NamedError("original_error", err),
			)
		}
		return err
	}

	return tx.Commit(ctx)
}
