package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ahmad/smart-inventory/config"
	appdb "github.com/ahmad/smart-inventory/db"
	"github.com/ahmad/smart-inventory/internal/auth"
	"github.com/ahmad/smart-inventory/internal/models"
	"github.com/ahmad/smart-inventory/internal/server"
	"gorm.io/gorm"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg := config.Load()

	db, err := appdb.Open(cfg)
	if err != nil {
		slog.Error("db open failed", "err", err)
		os.Exit(1)
	}

	if err := ensureAdmin(db, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		slog.Error("bootstrap admin failed", "err", err)
		os.Exit(1)
	}

	jwtMgr := auth.NewManager(cfg.JWTSecret, cfg.JWTTTLHours)
	r := server.NewRouter(db, jwtMgr)

	srv := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		slog.Info("http server listening", "addr", srv.Addr, "env", cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	slog.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown", "err", err)
	}
}

func ensureAdmin(db *gorm.DB, email, password string) error {
	if email == "" || password == "" {
		return nil
	}
	email = strings.ToLower(strings.TrimSpace(email))
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	var u models.User
	if err := db.Where("email = ?", email).First(&u).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		admin := &models.User{
			Email:        email,
			PasswordHash: hash,
			Name:         "Administrator",
			Role:         models.RoleAdmin,
		}
		if err := db.Create(admin).Error; err != nil {
			return err
		}
		slog.Info("bootstrap admin created", "email", email)
		return nil
	}

	// update password if env var changed
	if err := db.Model(&u).Update("password_hash", hash).Error; err != nil {
		return err
	}
	if u.Role != models.RoleAdmin {
		if err := db.Model(&u).Update("role", models.RoleAdmin).Error; err != nil {
			return err
		}
	}
	slog.Info("bootstrap admin updated", "email", email)
	return nil
}
