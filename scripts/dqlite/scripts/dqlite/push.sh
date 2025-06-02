#!/usr/bin/env bash

set -e

source "$(dirname $0)/../env.sh"

# s3 puts here are without an ACL.
# The bucket uses policies that allow GetObject to anyone,
# and PutObject to select Juju team accounts.

echo "Pushing ${S3_ARCHIVE_PATH} to s3"
aws s3 cp ${ARCHIVE_PATH} ${S3_ARCHIVE_PATH}

SUM=$(sha256sum ${FILE} | awk '{print $1}')
echo "Pushing ${SUM}.tar.bz2 to s3"
aws s3 cp ${S3_ARCHIVE_PATH} ${S3_BUCKET}/${SUM}.tar.bz2

# This is the old way and is deprecated.
echo "Pushing latest-dqlite-deps-${BUILD_ARCH}.tar.bz2 to s3"
aws s3 cp ${S3_ARCHIVE_PATH} ${S3_BUCKET}/latest-dqlite-deps-${BUILD_ARCH}.tar.bz2