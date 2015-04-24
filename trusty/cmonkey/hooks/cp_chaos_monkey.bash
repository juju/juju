#!/bin/bash -eux
chaos_dir="$(config-get chaos-dir)"
[ -d ${chaos_dir} ] && mv ${chaos_dir} ${chaos_dir}.$$
cp -a ${CHARM_DIR}/chaos-monkey ${chaos_dir}
mkdir -p ${chaos_dir}/log
chown -R ubuntu:ubuntu ${chaos_dir}
