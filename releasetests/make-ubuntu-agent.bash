#!/bin/bash
set -eux
RELEASE=$1
SERIES=$2
ARCH=$3
revision_build=$4
RELEASE_TOOLS=$(readlink -f $(dirname $0))
jujud=$(find -path ./juju-build-$SERIES-$ARCH/juju-core-*/bin/jujud -type f)
version=$(echo $jujud|sed -r "s/.\/juju-build-$SERIES-$ARCH\/juju-core-\
(.*)\/bin\/jujud/\1/")
tarfile=juju-$version-$SERIES-$ARCH.tgz
tar -czvf $tarfile  -C $(dirname $jujud) jujud
$RELEASE_TOOLS/make_agent_json.py ubuntu $tarfile $revision_build $version\
  $ARCH $RELEASE $SERIES
