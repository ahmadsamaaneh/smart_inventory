package db

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/ahmad/smart-inventory/config"
	"github.com/ahmad/smart-inventory/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open connects to the database, retrying if needed, then runs migrations.
func Open(cfg *config.Config) (*gorm.DB, error) {
	gormCfg := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	}

	gdb, err := openWithRetry(cfg, gormCfg)
	if err != nil {
		return nil, err
	}

	if err := tunePool(gdb, cfg.DB); err != nil {
		return nil, fmt.Errorf("tune pool: %w", err)
	}

	if err := AutoMigrate(gdb); err != nil {
		return nil, fmt.Errorf("auto-migrate: %w", err)
	}

	slog.Info("database ready", "driver", cfg.DB.Driver)
	return gdb, nil
}

func dial(driver, dsn string, gormCfg *gorm.Config) (*gorm.DB, error) {
	switch driver {
	case "postgres":
		return gorm.Open(postgres.Open(dsn), gormCfg)
	case "sqlite", "":
		return gorm.Open(sqlite.Open(dsn), gormCfg)
	default:
		return nil, fmt.Errorf("unsupported DB_DRIVER: %s", driver)
	}
}

func openWithRetry(cfg *config.Config, gormCfg *gorm.Config) (*gorm.DB, error) {
	retries := cfg.DB.ConnectRetries
	if retries < 1 {
		retries = 1
	}
	wait := time.Duration(cfg.DB.ConnectRetryWait) * time.Second
	if wait <= 0 {
		wait = 2 * time.Second
	}

	var lastErr error
	for attempt := 1; attempt <= retries; attempt++ {
		gdb, err := dial(cfg.DB.Driver, cfg.DB.DSN, gormCfg)
		if err == nil {
			// verify with a ping
			sqlDB, perr := gdb.DB()
			if perr == nil {
				if pingErr := sqlDB.Ping(); pingErr == nil {
					return gdb, nil
				} else {
					err = pingErr
				}
			} else {
				err = perr
			}
		}
		lastErr = err
		slog.Warn("db connect attempt failed",
			"attempt", attempt, "max", retries, "err", err,
		)
		if attempt < retries {
			time.Sleep(wait)
		}
	}
	return nil, fmt.Errorf("connect db after %d attempts: %w", retries, lastErr)
}

func tunePool(gdb *gorm.DB, cfg config.DBConfig) error {
	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}
	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifeMins > 0 {
		sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifeMins) * time.Minute)
	}
	return nil
}

// AutoMigrate keeps the schema in sync with our model structs.
func AutoMigrate(gdb *gorm.DB) error {
	return gdb.AutoMigrate(
		&models.User{},
		&models.Product{},
		&models.Order{},
		&models.OrderItem{},
		&models.StockMovement{},
	)
}
