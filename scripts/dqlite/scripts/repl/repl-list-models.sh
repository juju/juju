#!/bin/bash

set -e

MACHINE=${MACHINE:-0}

CMDS=$(cat << EOF
sudo awk '/controllercert/ {in_cert_block=1; next}
/:/ {in_cert_block=0}
in_cert_block { print }' /var/lib/juju/agents/machine-$MACHINE/agent.conf | sed 's/  //' > dqlite.cert
sudo awk '/controllerkey/ {in_cert_block=1; next}
/:/ {in_cert_block=0}
in_cert_block { print }' /var/lib/juju/agents/machine-$MACHINE/agent.conf | sed 's/  //' > dqlite.key
sudo dqlite -s file:///var/lib/juju/dqlite/cluster.yaml -c ./dqlite.cert -k ./dqlite.key controller "SELECT uuid, name FROM model"
EOF
)

juju ssh --pty=false -m controller ${MACHINE} "${CMDS}"
