#!/bin/bash

set -e

MACHINE=${MACHINE:-0}
DB_NAME=${DB_NAME:-controller}

echo "-------------------------------------------------------------------------"
echo ""
echo "DQLITE REPL Mode:"
echo ""
echo "Once logged in, run the following command:"
echo ""
echo " > dqlite -s \$(sudo cat /var/lib/juju/dqlite/info.yaml | grep \"Address: \" | cut -d\" \" -f2) ${DB_NAME}"
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

juju ssh -m controller ${MACHINE}
