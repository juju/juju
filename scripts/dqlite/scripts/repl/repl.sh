#!/bin/bash

set -e

MACHINE=${MACHINE:-0}
DB_NAME=${DB_NAME:-controller}

echo "-------------------------------------------------------------------------"
echo ""
echo "DQLITE REPL Mode: ${DB_NAME}"
echo ""
echo "-------------------------------------------------------------------------"
echo ""
echo "                             WARNING!"
echo ""
echo "You're attached to the live database. There currently is no audit trail"
echo "when running any commands. You could end up corrupting the database."
echo "Ensure you make a backup before running any commands."
echo ""
echo "--------------------------------------------------------------------------"
echo ""

CMDS=$(cat << EOF
sudo awk '/controllercert/ {in_cert_block=1; next}
/:/ {in_cert_block=0}
in_cert_block { print }' /var/lib/juju/agents/machine-$MACHINE/agent.conf | sed 's/  //' > dqlite.cert
sudo awk '/controllerkey/ {in_cert_block=1; next}
/:/ {in_cert_block=0}
in_cert_block { print }' /var/lib/juju/agents/machine-$MACHINE/agent.conf | sed 's/  //' > dqlite.key
sudo dqlite -s file:///var/lib/juju/dqlite/cluster.yaml -c ./dqlite.cert -k ./dqlite.key $DB_NAME
EOF
)

juju ssh -m controller ${MACHINE} "${CMDS}"
