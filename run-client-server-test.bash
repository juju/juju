#!/bin/bash
set -eu
SCRIPTS=$(readlink -f $(dirname $0))

usage() {
    echo "usage: $0 user@host old-version candidate-version new-to-old"
    exit 1
}
test $# -eq 4 || usage

user_at_host="$1"
old_version="$2"
candidate_version="$3"
new_to_old="$4"

set -x
cat > temp-config.yaml <<EOT
install:
  remote: [$SCRIPTS/run-client-server-test-remote.bash]
command: [remote/run-client-server-test-remote.bash $old_version $candidate_version $new_to_old]
EOT
workspace-run temp-config.yaml $user_at_host
