#!/usr/bin/env bash

set -e

source "$(dirname $0)/../env.sh"
source "$(dirname $0)/dqlite-build.sh"

TAG_LIBTIRPC=${TAG_LIBTIRPC} \
TAG_LIBNSL=${TAG_LIBNSL} \
TAG_LIBUV=${TAG_LIBUV} \
TAG_LIBLZ4=${TAG_LIBLZ4} \
TAG_RAFT=${TAG_RAFT} \
TAG_SQLITE=${TAG_SQLITE} \
TAG_DQLITE=${TAG_DQLITE} \
    build
