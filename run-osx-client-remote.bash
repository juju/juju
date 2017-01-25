#!/bin/bash
set -eu
export PATH=$HOME/juju-ci-tools:$PATH
WORKSPACE=$(pwd)
TARFILE=$1
revision_build=$2
source $HOME/.bashrc
set -x

env=testing-osx-client1
tar -xf $TARFILE -C $WORKSPACE
mkdir artifacts
deploy_job.py parallel-osx-client-base $WORKSPACE/juju-bin/juju artifacts $env \
    --series xenial --use-charmstore \
    --agent-stream=revision-build-$revision_build
# The host experiences connection issues with AWS, retry destroy just in case.
~/Bin/juju destroy-environment --force -y $env || true
