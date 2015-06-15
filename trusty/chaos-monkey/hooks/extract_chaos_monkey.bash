#!/bin/bash -eux
cd ${CHARM_DIR}
chaos_source="$(config-get chaos-source)"
wget ${chaos_source} --output-document chaos-monkey.tar.gz
# Delete any existing Chaos Monkey directory.
[[ -d chaos-monkey ]] && rm -rf chaos-monkey
# Extract and rename chaos-monkey-master to chaos-monkey
[[ -f chaos-monkey.tar.gz ]] && tar -zxf chaos-monkey.tar.gz && \
    mv chaos-monkey-master chaos-monkey
