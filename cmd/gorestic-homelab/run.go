package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/fgeck/gorestic-homelab/internal/config"
	"github.com/fgeck/gorestic-homelab/internal/services/runner"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Execute the backup workflow",
	Long: `Execute the complete backup workflow:
1. Wake-on-LAN (if configured)
2. Initialize restic repository (if needed)
3. PostgreSQL dump (if configured)
4. Backup to restic repository
5. Apply retention policy
6. Repository check (if enabled)
7. SSH shutdown (if configured)
8. Send Telegram notification (if configured)`,
	RunE: runBackup,
}

func runBackup(cmd *cobra.Command, args []string) error {
	if configFile == "" {
		log.Error().Msg("config file is required")
		return cmd.Help()
	}

	// Load configuration
	parser := config.NewParser()
	cfg, err := parser.LoadFile(configFile)
	if err != nil {
		log.Error().Err(err).Str("file", configFile).Msg("failed to load config")
		return err
	}

	// Validate configuration
	if err := config.Validate(cfg); err != nil {
		log.Error().Err(err).Msg("invalid configuration")
		return err
	}

	log.Info().
		Str("config", configFile).
		Str("repository", cfg.Restic.Repository).
		Str("host", cfg.Backup.Host).
		Msg("configuration loaded")

	// Set up context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Warn().Str("signal", sig.String()).Msg("received signal, shutting down")
		cancel()
	}()

	// Run backup
	runnerSvc := runner.New(log.Logger)
	if err := runnerSvc.Run(ctx, *cfg); err != nil {
		log.Error().Err(err).Msg("backup failed")
		return err
	}

	log.Info().Msg("backup completed successfully")
	return nil
}
