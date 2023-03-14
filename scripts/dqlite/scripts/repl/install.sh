#!/bin/bash

set -e

MACHINE=${MACHINE:-0}

juju exec -m controller --machine=${MACHINE} 'which dqlite &>/dev/null || sudo DEBIAN_FRONTEND=noninteractive add-apt-repository -y ppa:dqlite/dev && sudo DEBIAN_FRONTEND=noninteractive apt install -y dqlite-tools'
