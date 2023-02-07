#!/bin/bash

set -e

MACHINE=${MACHINE:-0}

juju exec -m controller --machine=${MACHINE} 'sudo which rlwrap &>/dev/null || sudo apt-get -y -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" install rlwrap socat'
