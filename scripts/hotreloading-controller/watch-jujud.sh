#!/bin/bash
#
# watch-jujud.sh - Watch for changes and rebuild jujud
#
# This script monitors the juju codebase for changes, automatically rebuilds
# jujud-controller when changes are detected, and runs a custom script when
# the build completes successfully.
#
# Usage:
#   ./scripts/watch-jujud.sh [path/to/post-build-script.sh]
#
# If no script is provided, it will just rebuild on changes.
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Script to run after successful build (optional)
POST_BUILD_SCRIPT="${1:-}"

# Directories to watch (adjust as needed)
WATCH_DIRS=(
    "agent"
    "api"
    "apiserver"
    "caas"
    "cmd"
    "core"
    "domain"
    "internal"
    "state"
    "worker"
)

# Check if inotify-tools is installed
if ! command -v inotifywait &> /dev/null; then
    echo -e "${RED}Error: inotifywait not found. Please install inotify-tools:${NC}"
    echo -e "${YELLOW}  sudo apt install inotify-tools${NC}"
    exit 1
fi

# Validate post-build script if provided
if [ -n "$POST_BUILD_SCRIPT" ]; then
    if [ ! -f "$POST_BUILD_SCRIPT" ]; then
        echo -e "${RED}Error: Post-build script not found: $POST_BUILD_SCRIPT${NC}"
        exit 1
    fi
    echo -e "${BLUE}Post-build script: $POST_BUILD_SCRIPT${NC}"
fi

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Watching juju codebase for changes...${NC}"
echo -e "${BLUE}Monitoring directories: ${WATCH_DIRS[*]}${NC}"
echo -e "${BLUE}Press Ctrl+C to stop${NC}"
echo -e "${BLUE}========================================${NC}\n"

# Function to build jujud
build_jujud() {
    echo -e "\n${YELLOW}[$(date '+%H:%M:%S')] Change detected, rebuilding jujud...${NC}"
    
    if make jujud-controller; then
        echo -e "${GREEN}[$(date '+%H:%M:%S')] ✓ Build successful!${NC}"
        
        # Run post-build script if provided
        if [ -n "$POST_BUILD_SCRIPT" ]; then
            echo -e "${BLUE}[$(date '+%H:%M:%S')] Running post-build script...${NC}"
            if "$POST_BUILD_SCRIPT"; then
                echo -e "${GREEN}[$(date '+%H:%M:%S')] ✓ Post-build script completed successfully${NC}"
            else
                echo -e "${RED}[$(date '+%H:%M:%S')] ✗ Post-build script failed${NC}"
            fi
        fi
    else
        echo -e "${RED}[$(date '+%H:%M:%S')] ✗ Build failed${NC}"
    fi
    
    echo -e "${BLUE}Waiting for changes...${NC}"
}

# Build once at startup
build_jujud

# Build watch directory list
WATCH_PATHS=""
for dir in "${WATCH_DIRS[@]}"; do
    if [ -d "$dir" ]; then
        WATCH_PATHS="$WATCH_PATHS $dir"
    fi
done

# Watch for changes
# -r: recursive
# -e: events to watch
# --exclude: exclude patterns
inotifywait -r -m -e modify,create,delete,move \
    --exclude '(_test\.go|\.swp|\.git|_build|\.tmp)' \
    $WATCH_PATHS \
    | while read -r directory event filename; do
        # Only trigger on .go files
        if [[ "$filename" == *.go ]]; then
            # Debounce: wait a bit to allow multiple rapid changes to settle
            sleep 0.5
            build_jujud
        fi
    done
