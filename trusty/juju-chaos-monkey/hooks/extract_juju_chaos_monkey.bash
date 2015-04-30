#!/bin/bash -eux
cd ${CHARM_DIR}
[[ -f juju-chaos-monkey.tgz ]] && tar -zxf juju-chaos-monkey.tgz
