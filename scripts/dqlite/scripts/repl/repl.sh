#!/bin/bash

set -e

MACHINE=${MACHINE:-0}

juju ssh -m controller ${MACHINE} 'sudo rlwrap -H /root/.dqlite_repl.history socat - /var/lib/juju/dqlite/juju.sock'
