// Comando seed: aplica migraciones y siembra datos base (3 empresas + roles + admin).
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gpvdp/erp/internal/config"
	"github.com/gpvdp/erp/internal/database"
	"github.com/gpvdp/erp/internal/logging"
	"github.com/gpvdp/erp/internal/seed"
	"github.com/gpvdp/erp/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seed error:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger, err := logging.New(cfg.Env)
	if err != nil {
		return err
	}
	defer func() { _ = logger.Sync() }()

	ctx := context.Background()

	if err := database.RunMigrations(migrations.FS, cfg.DatabaseURL); err != nil {
		return err
	}
	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	return seed.Run(ctx, pool, logger, cfg)
}
