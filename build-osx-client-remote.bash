#!/bin/bash
set -eu
revision_build=$1
RELEASE_SCRIPTS=$HOME/juju-release-tools
SCRIPTS=$HOME/juju-ci-tools
GOBASE=$HOME/crossbuild
WORKSPACE=$HOME/workspace
JUJU_HOME=$HOME/.juju
source $HOME/.bashrc
source $HOME/cloud-city/juju-qa.jujuci
set -x

cd $WORKSPACE
$SCRIPTS/jujuci.py -v setup-workspace $WORKSPACE
TARFILE=$($SCRIPTS/jujuci.py get build-revision 'juju-core_*.tar.gz' ./)
echo "Downloaded $TARFILE"
$RELEASE_SCRIPTS/crossbuild.py -v osx-client -b $GOBASE ./$TARFILE
