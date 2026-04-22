#!/bin/bash
set -e

# E2E Test Script for gorestic-homelab
# Tests: WOL -> Wait for Restic -> Backup -> SSH Shutdown

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
CONFIG_FILE="${PROJECT_DIR}/config.e2e-test.yaml"

echo "=========================================="
echo "  gorestic-homelab E2E Test"
echo "=========================================="
echo ""

# Check if config file exists
if [ ! -f "$CONFIG_FILE" ]; then
    echo "ERROR: Config file not found: $CONFIG_FILE"
    echo "Please copy config.e2e-test.yaml and fill in your values"
    exit 1
fi

# Check for required environment variables
if [ -z "$RESTIC_PASSWORD" ]; then
    echo "ERROR: RESTIC_PASSWORD environment variable is not set"
    echo "Please set it: export RESTIC_PASSWORD='your-password'"
    exit 1
fi

# Build the binary
echo "Step 1: Building gorestic-homelab..."
cd "$PROJECT_DIR"
go build -o gorestic-homelab ./cmd/gorestic-homelab
echo "  ✓ Build complete"
echo ""

# Validate configuration
echo "Step 2: Validating configuration..."
./gorestic-homelab validate --config "$CONFIG_FILE"
echo ""

# Ask for confirmation before running
echo "=========================================="
echo "  READY TO RUN E2E TEST"
echo "=========================================="
echo ""
echo "This will:"
echo "  1. Send WOL packet to wake target machine"
echo "  2. Wait for restic REST server to become available"
echo "  3. Initialize restic repository (if needed)"
echo "  4. Create a backup"
echo "  5. Apply retention policy"
echo "  6. Send SSH shutdown command to target"
echo ""
echo "The target machine will be SHUT DOWN after the test!"
echo ""
read -p "Are you sure you want to proceed? (yes/no): " confirm

if [ "$confirm" != "yes" ]; then
    echo "Aborted."
    exit 0
fi

# Create test data
echo ""
echo "Step 3: Creating test data..."
mkdir -p /tmp/e2e-test-backup
echo "E2E test data - $(date)" > /tmp/e2e-test-backup/test-file.txt
echo "  ✓ Test data created at /tmp/e2e-test-backup"
echo ""

# Run the backup
echo "Step 4: Running backup workflow..."
echo ""
./gorestic-homelab run --config "$CONFIG_FILE" --verbose

echo ""
echo "=========================================="
echo "  E2E TEST COMPLETE"
echo "=========================================="
echo ""
echo "The target machine should now be shutting down."
echo ""

# Cleanup
rm -rf /tmp/e2e-test-backup
echo "Test data cleaned up."
