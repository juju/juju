#!/usr/bin/env bash

set -e

source "$(dirname $0)/../env.sh"

BUILD_IMAGE=ubuntu:22.04
BUILD_CONTAINER=lib-build-server

lxc delete -f ${BUILD_CONTAINER} &>/dev/null || true

lxc launch ${BUILD_IMAGE} ${BUILD_CONTAINER}
lxc exec ${BUILD_CONTAINER} -- bash -c 'while [ "$(systemctl is-system-running 2>/dev/null)" != "running" ] && [ "$(systemctl is-system-running 2>/dev/null)" != "degraded" ]; do :; done'

lxc exec ${BUILD_CONTAINER} -- bash -c 'mkdir -p /root/dqlite'
lxc file push $(dirname $0)/../env.sh ${BUILD_CONTAINER}/root/env.sh
lxc file push $(dirname $0)/build.sh ${BUILD_CONTAINER}/root/dqlite/build.sh
lxc file push $(dirname $0)/dqlite-build.sh ${BUILD_CONTAINER}/root/dqlite/dqlite-build.sh

lxc exec -t ${BUILD_CONTAINER} bash /root/dqlite/build.sh

mkdir -p ${ARCHIVE_DEPS_PATH}
lxc file pull ${BUILD_CONTAINER}/root/_build/dqlite-deps.tar.bz2 ${ARCHIVE_PATH}
lxc delete -f ${BUILD_CONTAINER}
