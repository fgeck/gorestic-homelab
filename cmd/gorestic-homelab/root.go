package main

import (
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	// Version is set at build time.
	Version = "dev"

	// Configuration flags.
	configFile string
	verbose    bool
	quiet      bool
	jsonOutput bool
)

var rootCmd = &cobra.Command{
	Use:   "gorestic-homelab",
	Short: "A restic backup orchestrator for homelab environments",
	Long: `gorestic-homelab is a Go-based backup orchestrator that handles:
  - Wake-on-LAN to wake backup targets
  - PostgreSQL backups via pg_dump
  - Restic backup operations
  - SSH shutdown of remote servers
  - Telegram notifications

Use as a one-shot command with an external scheduler (cron, systemd timer, etc.)`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		setupLogging()
	},
	Version: Version,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "config file (required)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose (debug) output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "enable quiet mode (errors only)")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output logs in JSON format")

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(validateCmd)
}

func setupLogging() {
	// Set output format
	if jsonOutput {
		log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
	} else {
		output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "15:04:05"}
		output.FormatLevel = func(i interface{}) string {
			if s, ok := i.(string); ok {
				return strings.ToUpper(s)
			}
			return ""
		}
		log.Logger = zerolog.New(output).With().Timestamp().Logger()
	}

	// Set log level
	switch {
	case quiet:
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	case verbose:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
