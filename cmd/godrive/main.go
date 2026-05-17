package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"godrive/internal/auth"
	"godrive/internal/config"
	"godrive/internal/files"
	"godrive/internal/logging"
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
	logLevel := slog.LevelInfo
	if raw := os.Getenv("GODRIVE_LOG_LEVEL"); raw != "" {
		if err := logLevel.UnmarshalText([]byte(raw)); err != nil {
			logLevel = slog.LevelInfo
		}
	}
	log := slog.New(logging.New(os.Stdout, logLevel))
	slog.SetDefault(log)

	command := "serve"
	args := os.Args[1:]
	if len(args) > 0 {
		command = args[0]
		args = args[1:]
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if command != "serve" {
		return runCLI(context.Background(), command, args, cfg, log)
	}
	if err := cfg.EnsureDirs(); err != nil {
		return err
	}

	st, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer func() {
		if err := st.Close(); err != nil {
			log.Warn("failed to close store", "err", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := st.Migrate(ctx); err != nil {
		return err
	}
	if err := bootstrapAdmin(ctx, st, cfg, log); err != nil {
		return err
	}
	_ = st.DeleteExpiredSessions(ctx, time.Now().UTC())

	fileService := files.NewService(cfg.TrashDir, st)
	srv := server.New(cfg, st, fileService, log)

	if cfg.EnableWatcher {
		if watcher, err := watch.New(log, st); err != nil {
			log.Warn("filesystem watcher disabled", "err", err)
		} else {
			defer func() {
				if err := watcher.Close(); err != nil {
					log.Warn("failed to close watcher", "err", err)
				}
			}()
			users, err := st.ListUsers(ctx)
			if err != nil {
				log.Warn("failed to load roots for watcher", "err", err)
			} else {
				if err := watcher.SetUserRoots(users); err != nil {
					log.Warn("failed to watch user roots", "err", err)
				}
				go watcher.Run(ctx)
				srv.SetWatcher(watcher)
				srv.StartWatcherReconciliation(ctx)
			}
		}
	}
	srv.StartReconciliation(ctx, cfg.ReconcileInterval)
	srv.StartUploadCleanup(ctx, cfg.UploadDir, cfg.UploadTTL)
	srv.StartSessionCleanup(ctx)

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.Addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Warn("server shutdown error", "err", err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func runCLI(ctx context.Context, command string, args []string, cfg config.Config, log *slog.Logger) error {
	switch command {
	case "help", "-h", "--help":
		printUsage()
		return nil
	case "status":
		return cliStatus(ctx, cfg)
	case "verify":
		return cliVerify(cfg)
	case "reindex":
		return cliReindex(ctx, args, cfg, log)
	case "preview-warmup":
		return cliPreviewWarmup(ctx, cfg, log)
	case "preview-cache":
		return cliPreviewCache(args, cfg)
	case "uploads":
		return cliUploads(ctx, args, cfg, log)
	case "admin":
		return cliAdmin(ctx, args, cfg)
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func printUsage() {
	fmt.Println(`Usage:
  godrive serve
  godrive status
  godrive verify
  godrive reindex
  godrive reindex --user USER
  godrive reindex --user USER --path /subfolder
  godrive preview-warmup
  godrive preview-cache clear
  godrive uploads cleanup [--ttl 48h]
  godrive admin create --username USER --password PASS --root PATH [--admin]
  godrive admin reset-password --username USER --password PASS`)
}

func cliStatus(ctx context.Context, cfg config.Config) error {
	st, err := openMigratedStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	users, err := st.ListUsers(ctx)
	if err != nil {
		return err
	}
	indexStats, err := st.IndexStats(ctx)
	if err != nil {
		return err
	}
	trashCount, trashBytes, err := st.TrashStats(ctx)
	if err != nil {
		return err
	}
	previewFiles, previewBytes, err := directoryStats(cfg.PreviewDir)
	if err != nil {
		return err
	}

	fmt.Printf("database: %s\n", cfg.DatabasePath)
	fmt.Printf("users: %d\n", len(users))
	fmt.Printf("index: %d files, %d dirs, %d bytes, %d preview candidates\n", indexStats.IndexedFiles, indexStats.IndexedDirectories, indexStats.IndexedBytes, indexStats.PreviewCandidates)
	fmt.Printf("trash: %d items, %d bytes\n", trashCount, trashBytes)
	fmt.Printf("preview cache: %d files, %d bytes\n", previewFiles, previewBytes)
	return nil
}

func cliVerify(cfg config.Config) error {
	checks := []struct {
		name string
		path string
	}{
		{"data root", cfg.DataRoot},
		{"appdata", cfg.AppDataDir},
		{"database parent", filepath.Dir(cfg.DatabasePath)},
		{"uploads", cfg.UploadDir},
		{"previews", cfg.PreviewDir},
		{"trash", cfg.TrashDir},
		{"bootstrap admin root", cfg.BootstrapAdminRoot},
	}
	for _, check := range checks {
		if check.path == "" {
			return fmt.Errorf("%s path is empty", check.name)
		}
		if err := os.MkdirAll(check.path, 0o750); err != nil {
			return fmt.Errorf("%s %q: %w", check.name, check.path, err)
		}
		info, err := os.Stat(check.path)
		if err != nil {
			return fmt.Errorf("%s %q: %w", check.name, check.path, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%s %q is not a directory", check.name, check.path)
		}
		fmt.Printf("ok: %s %s\n", check.name, check.path)
	}
	for _, tool := range server.PreviewToolStatuses() {
		if tool.Available {
			fmt.Printf("ok: preview tool %s %s\n", tool.Name, tool.Path)
			continue
		}
		fmt.Printf("warn: preview tool %s missing (%s)\n", tool.Name, tool.Purpose)
	}
	return nil
}

func cliReindex(ctx context.Context, args []string, cfg config.Config, log *slog.Logger) error {
	fs := flag.NewFlagSet("reindex", flag.ContinueOnError)
	username := fs.String("user", "", "username to reindex; empty reindexes all users")
	logicalPath := fs.String("path", "", "logical file or folder path to repair; requires --user")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *logicalPath != "" && *username == "" {
		return errors.New("usage: godrive reindex --user USER --path /subfolder")
	}

	if err := cfg.EnsureDirs(); err != nil {
		return err
	}
	st, srv, err := maintenanceServer(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	var job *server.AdminJob
	if *logicalPath != "" {
		job, err = srv.RunReindexPath(ctx, *username, *logicalPath)
	} else if *username != "" {
		job, err = srv.RunReindexUser(ctx, *username)
	} else {
		job, err = srv.RunReindex(ctx)
	}
	if err != nil {
		return err
	}
	printJob(job)
	return nil
}

func cliPreviewWarmup(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	if err := cfg.EnsureDirs(); err != nil {
		return err
	}
	st, srv, err := maintenanceServer(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	job, err := srv.RunPreviewWarmup(ctx)
	if err != nil {
		return err
	}
	printJob(job)
	return nil
}

func cliPreviewCache(args []string, cfg config.Config) error {
	if len(args) != 1 || args[0] != "clear" {
		return errors.New("usage: godrive preview-cache clear")
	}
	if err := cfg.ValidatePreviewCacheDir(); err != nil {
		return errors.New("invalid preview cache directory")
	}
	if err := os.RemoveAll(cfg.PreviewDir); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.PreviewDir, 0o750); err != nil {
		return err
	}
	fmt.Printf("cleared preview cache: %s\n", cfg.PreviewDir)
	return nil
}

func cliUploads(ctx context.Context, args []string, cfg config.Config, log *slog.Logger) error {
	if len(args) == 0 || args[0] != "cleanup" {
		return errors.New("usage: godrive uploads cleanup [--ttl 48h]")
	}
	fs := flag.NewFlagSet("uploads cleanup", flag.ContinueOnError)
	rawTTL := fs.String("ttl", cfg.UploadTTL.String(), "remove unfinished uploads older than this duration")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	ttl, err := time.ParseDuration(*rawTTL)
	if err != nil {
		return err
	}
	if ttl <= 0 {
		return errors.New("ttl must be positive")
	}
	if err := cfg.EnsureDirs(); err != nil {
		return err
	}
	st, srv, err := maintenanceServer(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	result, err := srv.RunUploadCleanup(ctx, cfg.UploadDir, ttl)
	if err != nil {
		return err
	}
	fmt.Printf("upload cleanup: expired_records=%d expired_files=%d orphaned_files=%d\n", result.ExpiredRecords, result.ExpiredFiles, result.OrphanedFiles)
	return nil
}

func cliAdmin(ctx context.Context, args []string, cfg config.Config) error {
	if len(args) == 0 {
		return errors.New("usage: godrive admin create|reset-password")
	}
	switch args[0] {
	case "create":
		return cliAdminCreate(ctx, args[1:], cfg)
	case "reset-password":
		return cliAdminResetPassword(ctx, args[1:], cfg)
	default:
		return fmt.Errorf("unknown admin command %q", args[0])
	}
}

func cliAdminCreate(ctx context.Context, args []string, cfg config.Config) error {
	fs := flag.NewFlagSet("admin create", flag.ContinueOnError)
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	root := fs.String("root", "", "home root")
	isAdmin := fs.Bool("admin", true, "grant admin privileges")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *username == "" || *password == "" || *root == "" {
		return errors.New("usage: godrive admin create --username USER --password PASS --root PATH [--admin=false]")
	}
	if err := os.MkdirAll(*root, 0o750); err != nil {
		return err
	}

	st, err := openMigratedStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	hash, err := auth.HashPassword(*password)
	if err != nil {
		return err
	}
	user, err := st.CreateUser(ctx, store.User{
		Username:     *username,
		PasswordHash: hash,
		IsAdmin:      *isAdmin,
		HomeRoot:     *root,
	})
	if err != nil {
		return err
	}
	fmt.Printf("created user %s id=%d admin=%t root=%s\n", user.Username, user.ID, user.IsAdmin, user.HomeRoot)
	return nil
}

func cliAdminResetPassword(ctx context.Context, args []string, cfg config.Config) error {
	fs := flag.NewFlagSet("admin reset-password", flag.ContinueOnError)
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *username == "" || *password == "" {
		return errors.New("usage: godrive admin reset-password --username USER --password PASS")
	}

	st, err := openMigratedStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	user, err := st.GetUserByUsername(ctx, *username)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("user %q not found", *username)
		}
		return err
	}
	hash, err := auth.HashPassword(*password)
	if err != nil {
		return err
	}
	if err := st.SetPassword(ctx, user.ID, hash); err != nil {
		return err
	}
	fmt.Printf("reset password for %s id=%d\n", user.Username, user.ID)
	return nil
}

func openMigratedStore(ctx context.Context, cfg config.Config) (*store.Store, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.DatabasePath), 0o750); err != nil {
		return nil, err
	}
	st, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	if err := st.Migrate(ctx); err != nil {
		_ = st.Close()
		return nil, err
	}
	return st, nil
}

func maintenanceServer(ctx context.Context, cfg config.Config, log *slog.Logger) (*store.Store, *server.Server, error) {
	st, err := openMigratedStore(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}
	fileService := files.NewService(cfg.TrashDir, st)
	return st, server.New(cfg, st, fileService, log), nil
}

func printJob(job *server.AdminJob) {
	if job == nil {
		fmt.Println("job: none")
		return
	}
	fmt.Printf("job %s %s: %s, done=%d", job.ID, job.Type, job.Status, job.Done)
	if job.TotalKnown {
		fmt.Printf("/%d", job.Total)
	}
	fmt.Printf(", failed=%d, deleted=%d", job.Failed, job.Deleted)
	if job.User != "" {
		fmt.Printf(", user=%s", job.User)
	}
	if job.Scope != "" {
		fmt.Printf(", scope=%s", job.Scope)
	}
	fmt.Printf(", message=%s\n", job.Message)
}

func directoryStats(root string) (filesCount int64, bytes int64, err error) {
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		filesCount++
		bytes += info.Size()
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, nil
	}
	return filesCount, bytes, err
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
