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

curl -L -o ${WORK}/juju.deb ${SRC}
mkdir ${WORK}/juju
dpkg-deb -e ${WORK}/juju.deb ${WORK}/juju
VERSION=$(sed -n 's/^Version: \([0-9]\+\).\([0-9]\+\).\([0-9]\+\)-[0-9]\+~\([0-9]\+\)~\([a-Z]\+\).*/\1.\2.\3/p' ${WORK}/juju/control)
if [ "${VERSION}" == "" ] ; then
	echo "cannot extract deb version"
	exit 2
fi

SERIES=$(sed -n 's/^Version: \([0-9]\+\).\([0-9]\+\).\([0-9]\+\)-[0-9]\+~\([0-9]\+\)~\([a-Z]\+\).*/\5/p' ${WORK}/juju/control)
case "${SERIES}" in 
	"precise" | "quantal" | "raring" | "saucy" )
		;;
	*)
		echo "invalid series"
		exit 2
		;;
esac

ARCH=$(sed -n 's/^Architecture: \([a-z]\+\)/\1/p' ${WORK}/juju/control)
case "${ARCH}" in 
	"amd64" | "i386" | "armel" | "armhf" )
		;;
	*)
		echo "invalid arch"
		exit 2
		;;
esac

TOOLS=${WORK}/juju-${VERSION}-${SERIES}-${ARCH}.tgz
dpkg-deb -x ${WORK}/juju.deb ${WORK}/juju
tar cvfz $TOOLS -C ${WORK}/juju/usr/bin jujud
s3up --public ${TOOLS} juju-dist/tools/
