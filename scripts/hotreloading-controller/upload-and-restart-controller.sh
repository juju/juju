#!/bin/bash
#
# example-post-build.sh - Example post-build script for watch-jujud.sh
#
# This is an example script that runs after jujud is successfully compiled.
# Customize this to perform any actions you need after the build completes.
#

set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}Running post-build tasks...${NC}"

# Example 1: Show the build info
JUJUD_PATH="$(go env GOPATH)/bin/jujud"
if [ -f "$JUJUD_PATH" ]; then
    echo -e "${GREEN}✓ jujud binary location: $JUJUD_PATH${NC}"
    ls -lh "$JUJUD_PATH"
fi

juju scp -m controller /home/duttos/go/bin/jujud 0:/home/ubuntu/jujud
juju exec -m controller --machine 0 "sudo mv /home/ubuntu/jujud /var/lib/juju/tools/machine-0/jujud"
juju exec -m controller --machine 0 "sudo systemctl restart jujud-machine-0.service" || true

echo -e "${GREEN}✓ Post-build tasks completed${NC}"
