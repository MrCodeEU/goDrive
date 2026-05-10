package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"godrive/internal/auth"
	"godrive/internal/config"
	"godrive/internal/files"
	"godrive/internal/server"
	"godrive/internal/store"
	"godrive/internal/watch"
)

func main() {
	if err := run(); err != nil {
		slog.Error("godrive exited", "err", err)
		os.Exit(1)
	}
}

func run() error {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(log)

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.EnsureDirs(); err != nil {
		return err
	}

	st, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := st.Migrate(ctx); err != nil {
		return err
	}
	if err := bootstrapAdmin(ctx, st, cfg, log); err != nil {
		return err
	}
	_ = st.DeleteExpiredSessions(ctx, time.Now().UTC())

	if cfg.EnableWatcher {
		if watcher, err := watch.New(log); err != nil {
			log.Warn("filesystem watcher disabled", "err", err)
		} else {
			defer watcher.Close()
			users, err := st.ListUsers(ctx)
			if err != nil {
				log.Warn("failed to load roots for watcher", "err", err)
			} else {
				for _, user := range users {
					if user.Disabled {
						continue
					}
					if err := watcher.AddRecursive(user.HomeRoot); err != nil {
						log.Warn("failed to watch user root", "user", user.Username, "root", user.HomeRoot, "err", err)
					}
				}
				go watcher.Run(ctx)
			}
		}
	}

	fileService := files.NewService(cfg.TrashDir, st)
	srv := server.New(cfg, st, fileService, log)

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func bootstrapAdmin(ctx context.Context, st *store.Store, cfg config.Config, log *slog.Logger) error {
	adminCount, err := st.CountAdmins(ctx)
	if err != nil {
		return err
	}
	if adminCount > 0 {
		return nil
	}
	if cfg.BootstrapAdminPassword == "" {
		log.Warn("no admin exists; set GODRIVE_BOOTSTRAP_ADMIN_PASSWORD for first boot")
		return nil
	}

	passwordHash, err := auth.HashPassword(cfg.BootstrapAdminPassword)
	if err != nil {
		return err
	}
	user, err := st.CreateUser(ctx, store.User{
		Username:     cfg.BootstrapAdminUser,
		PasswordHash: passwordHash,
		IsAdmin:      true,
		HomeRoot:     cfg.BootstrapAdminRoot,
	})
	if err != nil {
		return err
	}
	log.Info("bootstrapped admin user", "username", user.Username, "home_root", user.HomeRoot)
	return nil
}
