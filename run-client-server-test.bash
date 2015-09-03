#!/bin/bash
set -eux
SCRIPTS=$(readlink -f $(dirname $0))
export PATH=$HOME/workspace-runner:$PATH

usage() {
    echo "usage: $0 user@host server client agent-arg agent-value remote-script"
    exit 1
}
test $# -eq 6 || usage

user_at_host="$1"
server="$2"
client="$3"
agent_arg="$4"
agent_value="$5"
remote_script="$6"

cat > temp-config.yaml <<EOT
install:
    remote:
        - $SCRIPTS/$remote_script
        - "$server"
        - "$client"
command: [remote/$remote_script,
          "remote/$(basename $server)", "remote/$(basename $client)",
          "$agent_arg", "$agent_value"]
EOT

workspace-run temp-config.yaml $user_at_host
