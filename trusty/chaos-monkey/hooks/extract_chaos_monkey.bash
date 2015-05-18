#!/bin/bash -eux
cd ${CHARM_DIR}
[[ -f chaos-monkey.tgz ]] && tar -zxf chaos-monkey.tgz
