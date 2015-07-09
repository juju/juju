#!/bin/bash
# This script presumes ~/ci and ~/.juju is setup on the remote machine.
set -eu
SCRIPTS=$(readlink -f $(dirname $0))

usage() {
    echo "usage: $0 user@host revsion_build"
    echo "  user@host: The user and host to ssh to."
    exit 1
}


test $# -eq 2 || usage
USER_AT_HOST="$1"
revision_build="$2"

set -x
$SCRIPTS/jujuci.py get-build-vars --summary $revision_build

cat > temp-config.yaml <<EOT
install:
  remote:
    - $SCRIPTS/run-osx-client-remote.bash
command: [remote/run-osx-client-remote.bash]
EOT
workspace-run temp-config.yaml $USER_AT_HOST
