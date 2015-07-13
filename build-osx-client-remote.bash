#!/bin/bash
set -eu
RELEASE_SCRIPTS=$HOME/juju-release-tools
SCRIPTS=$HOME/juju-ci-tools
GOBASE=$HOME/crossbuild
source $HOME/.bashrc
source $HOME/cloud-city/juju-qa.jujuci
set -x

TARFILE=$($SCRIPTS/jujuci.py get build-revision 'juju-core_*.tar.gz' ./)
echo "Downloaded $TARFILE"
$RELEASE_SCRIPTS/crossbuild.py -v osx-client -b $GOBASE ./$TARFILE
