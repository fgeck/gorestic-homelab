package restic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/fgeck/gorestic-homelab/internal/models"
	"github.com/rs/zerolog"
)

// Service defines the interface for restic operations.
type Service interface {
	Init(ctx context.Context, cfg models.ResticConfig) error
	Snapshots(ctx context.Context, cfg models.ResticConfig) ([]models.Snapshot, error)
	Backup(ctx context.Context, cfg models.ResticConfig, settings models.BackupSettings) (*models.BackupResult, error)
	Forget(ctx context.Context, cfg models.ResticConfig, policy models.RetentionPolicy) (*models.ForgetResult, error)
	Check(ctx context.Context, cfg models.ResticConfig, settings models.CheckSettings) (*models.CheckResult, error)
}

// CommandExecutor allows mocking exec.Command in tests.
type CommandExecutor interface {
	Execute(ctx context.Context, name string, args ...string) ([]byte, error)
	ExecuteWithEnv(ctx context.Context, env []string, name string, args ...string) ([]byte, error)
}

// DefaultExecutor is the default command executor using os/exec.
type DefaultExecutor struct{}

// Execute runs a command and returns its output.
func (e *DefaultExecutor) Execute(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// ExecuteWithEnv runs a command with additional environment variables.
func (e *DefaultExecutor) ExecuteWithEnv(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), env...)
	return cmd.CombinedOutput()
}

// Impl implements the Service interface.
type Impl struct {
	executor CommandExecutor
	logger   zerolog.Logger
}

// New creates a new restic service.
func New(logger zerolog.Logger) *Impl {
	return &Impl{
		executor: &DefaultExecutor{},
		logger:   logger,
	}
}

// NewWithExecutor creates a new restic service with a custom executor (for testing).
func NewWithExecutor(logger zerolog.Logger, executor CommandExecutor) *Impl {
	return &Impl{
		executor: executor,
		logger:   logger,
	}
}

func (s *Impl) buildEnv(cfg models.ResticConfig) []string {
	env := []string{
		fmt.Sprintf("RESTIC_REPOSITORY=%s", cfg.Repository),
		fmt.Sprintf("RESTIC_PASSWORD=%s", cfg.Password),
	}

	if cfg.RestUser != "" {
		env = append(env, fmt.Sprintf("RESTIC_REST_USERNAME=%s", cfg.RestUser))
	}
	if cfg.RestPassword != "" {
		env = append(env, fmt.Sprintf("RESTIC_REST_PASSWORD=%s", cfg.RestPassword))
	}

	return env
}

// Init initializes a restic repository if it doesn't exist.
func (s *Impl) Init(ctx context.Context, cfg models.ResticConfig) error {
	s.logger.Info().Str("repository", cfg.Repository).Msg("checking if repository needs initialization")

	env := s.buildEnv(cfg)

	// Check if repository already exists by running snapshots
	_, err := s.executor.ExecuteWithEnv(ctx, env, "restic", "snapshots", "--json")
	if err == nil {
		s.logger.Info().Msg("repository already initialized")
		return nil
	}

	// Initialize repository
	s.logger.Info().Msg("initializing repository")
	output, err := s.executor.ExecuteWithEnv(ctx, env, "restic", "init")
	if err != nil {
		return fmt.Errorf("failed to initialize repository: %w, output: %s", err, string(output))
	}

	s.logger.Info().Msg("repository initialized successfully")
	return nil
}

// snapshotJSON is the JSON structure returned by restic snapshots --json.
type snapshotJSON struct {
	ID       string    `json:"id"`
	Time     time.Time `json:"time"`
	Hostname string    `json:"hostname"`
	Tags     []string  `json:"tags"`
	Paths    []string  `json:"paths"`
}

// Snapshots returns a list of snapshots in the repository.
func (s *Impl) Snapshots(ctx context.Context, cfg models.ResticConfig) ([]models.Snapshot, error) {
	s.logger.Debug().Msg("listing snapshots")

	env := s.buildEnv(cfg)
	output, err := s.executor.ExecuteWithEnv(ctx, env, "restic", "snapshots", "--json")
	if err != nil {
		return nil, fmt.Errorf("failed to list snapshots: %w, output: %s", err, string(output))
	}

	var snapshots []snapshotJSON
	if err := json.Unmarshal(output, &snapshots); err != nil {
		return nil, fmt.Errorf("failed to parse snapshots: %w", err)
	}

	result := make([]models.Snapshot, len(snapshots))
	for i, snap := range snapshots {
		result[i] = models.Snapshot{
			ID:       snap.ID,
			Time:     snap.Time,
			Hostname: snap.Hostname,
			Tags:     snap.Tags,
			Paths:    snap.Paths,
		}
	}

	s.logger.Debug().Int("count", len(result)).Msg("snapshots listed")
	return result, nil
}

// backupSummary is the summary part of restic backup --json output.
type backupSummary struct {
	MessageType         string  `json:"message_type"`
	FilesNew            int     `json:"files_new"`
	FilesChanged        int     `json:"files_changed"`
	FilesUnmodified     int     `json:"files_unmodified"`
	DataAdded           int64   `json:"data_added"`
	TotalFilesProcessed int     `json:"total_files_processed"`
	TotalBytesProcessed int64   `json:"total_bytes_processed"`
	TotalDuration       float64 `json:"total_duration"`
	SnapshotID          string  `json:"snapshot_id"`
}

// Backup performs a backup operation.
func (s *Impl) Backup(ctx context.Context, cfg models.ResticConfig, settings models.BackupSettings) (*models.BackupResult, error) {
	s.logger.Info().Strs("paths", settings.Paths).Msg("starting backup")

	start := time.Now()
	env := s.buildEnv(cfg)

	args := []string{"backup", "--json"}

	// Add hostname
	if settings.Host != "" {
		args = append(args, "--host", settings.Host)
	}

	// Add tags
	for _, tag := range settings.Tags {
		args = append(args, "--tag", tag)
	}

	// Add paths
	args = append(args, settings.Paths...)

	output, err := s.executor.ExecuteWithEnv(ctx, env, "restic", args...)
	if err != nil {
		return &models.BackupResult{
			Duration: time.Since(start),
			Error:    fmt.Errorf("backup failed: %w, output: %s", err, string(output)),
		}, nil
	}

	// Parse the JSON output to find the summary line
	var summary backupSummary
	lines := bytes.Split(output, []byte("\n"))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var msg struct {
			MessageType string `json:"message_type"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		if msg.MessageType == "summary" {
			if err := json.Unmarshal(line, &summary); err != nil {
				s.logger.Warn().Err(err).Msg("failed to parse backup summary")
			}
			break
		}
	}

	result := &models.BackupResult{
		SnapshotID:          summary.SnapshotID,
		FilesNew:            summary.FilesNew,
		FilesChanged:        summary.FilesChanged,
		FilesUnmodified:     summary.FilesUnmodified,
		DataAdded:           summary.DataAdded,
		TotalFilesProcessed: summary.TotalFilesProcessed,
		TotalBytesProcessed: summary.TotalBytesProcessed,
		Duration:            time.Since(start),
	}

	s.logger.Info().
		Str("snapshot_id", result.SnapshotID).
		Int("files_new", result.FilesNew).
		Int("files_changed", result.FilesChanged).
		Int64("data_added", result.DataAdded).
		Dur("duration", result.Duration).
		Msg("backup completed")

	return result, nil
}

// forgetGroup is the JSON structure returned by restic forget --json.
type forgetGroup struct {
	Keep   []snapshotJSON `json:"keep"`
	Remove []snapshotJSON `json:"remove"`
}

// Forget removes old snapshots according to the retention policy.
func (s *Impl) Forget(ctx context.Context, cfg models.ResticConfig, policy models.RetentionPolicy) (*models.ForgetResult, error) {
	s.logger.Info().
		Int("keep_daily", policy.KeepDaily).
		Int("keep_weekly", policy.KeepWeekly).
		Int("keep_monthly", policy.KeepMonthly).
		Msg("applying retention policy")

	start := time.Now()
	env := s.buildEnv(cfg)

	args := []string{"forget", "--prune", "--json"}

	if policy.KeepDaily > 0 {
		args = append(args, "--keep-daily", fmt.Sprintf("%d", policy.KeepDaily))
	}
	if policy.KeepWeekly > 0 {
		args = append(args, "--keep-weekly", fmt.Sprintf("%d", policy.KeepWeekly))
	}
	if policy.KeepMonthly > 0 {
		args = append(args, "--keep-monthly", fmt.Sprintf("%d", policy.KeepMonthly))
	}

	output, err := s.executor.ExecuteWithEnv(ctx, env, "restic", args...)
	if err != nil {
		return &models.ForgetResult{
			Duration: time.Since(start),
			Error:    fmt.Errorf("forget failed: %w, output: %s", err, string(output)),
		}, nil
	}

	// Parse output to count kept/removed snapshots
	var groups []forgetGroup
	if err := json.Unmarshal(output, &groups); err != nil {
		// If the output is empty or not valid JSON, that's okay
		s.logger.Debug().Err(err).Msg("could not parse forget output")
	}

	result := &models.ForgetResult{
		Duration: time.Since(start),
	}

	for _, group := range groups {
		result.SnapshotsKept += len(group.Keep)
		result.SnapshotsRemoved += len(group.Remove)
	}

	s.logger.Info().
		Int("kept", result.SnapshotsKept).
		Int("removed", result.SnapshotsRemoved).
		Dur("duration", result.Duration).
		Msg("retention policy applied")

	return result, nil
}

// Check verifies the repository integrity.
func (s *Impl) Check(ctx context.Context, cfg models.ResticConfig, settings models.CheckSettings) (*models.CheckResult, error) {
	if !settings.Enabled {
		return &models.CheckResult{Passed: true}, nil
	}

	s.logger.Info().Str("subset", settings.Subset).Msg("checking repository")

	start := time.Now()
	env := s.buildEnv(cfg)

	args := []string{"check"}
	if settings.Subset != "" {
		args = append(args, "--read-data-subset", settings.Subset)
	}

	output, err := s.executor.ExecuteWithEnv(ctx, env, "restic", args...)
	duration := time.Since(start)

	if err != nil {
		// Check if it's just warnings or actual errors
		outputStr := strings.ToLower(string(output))
		if strings.Contains(outputStr, "error") {
			return &models.CheckResult{
				Passed:   false,
				Duration: duration,
				Error:    fmt.Errorf("check failed: %w, output: %s", err, string(output)),
			}, nil
		}
	}

	s.logger.Info().Str("duration", duration.Round(time.Millisecond).String()).Msg("repository check completed")

	return &models.CheckResult{
		Passed:   true,
		Duration: duration,
	}, nil
}
