#!/bin/bash

# release-public-tools consumes the release dpkg and uploads
# a matching set of tools to the juju-dist public bucket.

set -e

function usage {
	echo "usage $RELEASE_DEB_URL"
}

if [ $# -ne 1 ] ; then
	usage
	exit 1
fi

SRC=$1
WORK=$(mktemp -d)
SERIES=precise
ARCH=amd64

curl -L -o ${WORK}/juju.deb ${SRC}
mkdir ${WORK}/juju
dpkg-deb -e ${WORK}/juju.deb ${WORK}/juju
VERSION=$(sed -n 's/^Version: \([0-9]\+\).\([0-9]\+\).\([0-9]\+\).*/\1.\2.\3/p' ${WORK}/juju/control)
if [ ${VERSION} == "" ] ; then
	echo "cannot extract deb version"
	exit 2
fi

dpkg-deb -x ${WORK}/juju.deb ${WORK}/juju

TOOLS=${WORK}/juju-${VERSION}-${SERIES}-${ARCH}.tgz
tar cvfz $TOOLS -C ${WORK}/juju/usr/bin jujuc jujud
s3up --public ${TOOLS} juju-dist/tools/
