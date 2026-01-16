package main

import (
	"fmt"
	"os"

	"github.com/fgeck/gorestic-homelab/internal/config"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration file",
	Long:  `Validate the configuration file without executing any backup operations.`,
	RunE:  validateConfig,
}

func validateConfig(cmd *cobra.Command, args []string) error {
	if configFile == "" {
		log.Error().Msg("config file is required")
		return cmd.Help()
	}

	// Check if file exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		log.Error().Str("file", configFile).Msg("config file not found")
		return fmt.Errorf("config file not found: %s", configFile)
	}

	// Load configuration
	parser := config.NewParser()
	cfg, err := parser.LoadFile(configFile)
	if err != nil {
		log.Error().Err(err).Str("file", configFile).Msg("failed to parse config")
		return err
	}

	// Validate configuration
	if err := config.Validate(cfg); err != nil {
		log.Error().Err(err).Msg("configuration validation failed")
		return err
	}

	// Print configuration summary
	fmt.Println("Configuration is valid!")
	fmt.Println()
	fmt.Println("Summary:")
	fmt.Printf("  Repository: %s\n", cfg.Restic.Repository)
	fmt.Printf("  Host: %s\n", cfg.Backup.Host)
	fmt.Printf("  Paths: %v\n", cfg.Backup.Paths)
	fmt.Printf("  Tags: %v\n", cfg.Backup.Tags)
	fmt.Println()
	fmt.Println("Retention Policy:")
	fmt.Printf("  Keep daily: %d\n", cfg.Retention.KeepDaily)
	fmt.Printf("  Keep weekly: %d\n", cfg.Retention.KeepWeekly)
	fmt.Printf("  Keep monthly: %d\n", cfg.Retention.KeepMonthly)
	fmt.Println()
	fmt.Println("Optional Features:")
	fmt.Printf("  Wake-on-LAN: %v\n", cfg.WOL != nil)
	fmt.Printf("  PostgreSQL: %v\n", cfg.Postgres != nil)
	fmt.Printf("  SSH Shutdown: %v\n", cfg.SSHShutdown != nil)
	fmt.Printf("  Telegram: %v\n", cfg.Telegram != nil)
	fmt.Printf("  Repository Check: %v\n", cfg.Check.Enabled)

	if cfg.WOL != nil {
		fmt.Println()
		fmt.Println("WOL Configuration:")
		fmt.Printf("  MAC Address: %s\n", cfg.WOL.MACAddress)
		fmt.Printf("  Broadcast IP: %s\n", cfg.WOL.BroadcastIP)
		if cfg.WOL.TargetURL != "" {
			fmt.Printf("  Target URL: %s\n", cfg.WOL.TargetURL)
		}
	}

	if cfg.Postgres != nil {
		fmt.Println()
		fmt.Println("PostgreSQL Configuration:")
		fmt.Printf("  Host: %s\n", cfg.Postgres.Host)
		fmt.Printf("  Port: %d\n", cfg.Postgres.Port)
		fmt.Printf("  Database: %s\n", cfg.Postgres.Database)
		fmt.Printf("  Format: %s\n", cfg.Postgres.Format)
	}

	if cfg.SSHShutdown != nil {
		fmt.Println()
		fmt.Println("SSH Shutdown Configuration:")
		fmt.Printf("  Host: %s\n", cfg.SSHShutdown.Host)
		fmt.Printf("  Port: %d\n", cfg.SSHShutdown.Port)
		fmt.Printf("  Username: %s\n", cfg.SSHShutdown.Username)
		fmt.Printf("  OS: %s\n", cfg.SSHShutdown.OS)
		fmt.Printf("  Shutdown Delay: %d minute(s)\n", cfg.SSHShutdown.ShutdownDelay)
	}

	if cfg.Telegram != nil {
		fmt.Println()
		fmt.Println("Telegram Configuration:")
		fmt.Printf("  Chat ID: %s\n", cfg.Telegram.ChatID)
		fmt.Printf("  Bot Token: (configured)\n")
	}

	if cfg.Check.Enabled {
		fmt.Println()
		fmt.Println("Check Configuration:")
		fmt.Printf("  Subset: %s\n", cfg.Check.Subset)
	}

	return nil
}
