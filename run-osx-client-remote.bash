#!/bin/bash
set -eu
export PATH=$HOME/juju-ci-tools:$PATH
WORKSPACE=$(pwd)
JUJU_HOME=$HOME/.juju
source $HOME/.bashrc
source $HOME/cloud-city/juju-qa.jujuci
set -x

jujuci.py setup-workspace --clean-env testing-osx-client $WORKSPACE
~/Bin/juju destroy-environment --force -y testing-osx-client || true
TARFILE=$(jujuci.py get build-osx-client 'juju-*-osx.tar.gz' ./)
echo "Downloaded $TARFILE"
tar -xf ./$TARFILE -C $WORKSPACE

deploy_job.py testing-osx-client artifacts testing-osx-client \
  --new-juju-bin $WORKSPACE/juju-bin
