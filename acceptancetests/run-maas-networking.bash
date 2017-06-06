#!/bin/bash
# This script presumes ~/ci and ~/.juju is setup on the remote machine.
set -eu
SCRIPTS=$(readlink -f $(dirname $0))

usage() {
    echo "usage: $0 user@host tarball"
    echo "  user@host: The user and host to ssh to."
    exit 1
}

test $# -eq 2 || usage
USER_AT_HOST="$1"
TARFILE="$2"

set -x

cat > temp-config.yaml <<EOT
install:
  remote:
    - $SCRIPTS/run-maas-networking-remote.bash
    - "$TARFILE"
command: [remote/run-maas-networking-remote.bash, "remote/$(basename $TARFILE)"]
EOT
~/workspace-runner/workspace-run temp-config.yaml $USER_AT_HOST
