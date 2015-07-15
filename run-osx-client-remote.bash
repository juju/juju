#!/bin/bash
set -eu
export PATH=$HOME/juju-ci-tools:$PATH
WORKSPACE=$(pwd)
TARFILE=$1
source $HOME/.bashrc
set -x

env=testing-osx-client1
~/Bin/juju destroy-environment --force -y $env || true
tar -xf $TARFILE -C $WORKSPACE
mkdir artifacts
deploy_job.py testing-osx-client-base $WORKSPACE/juju-bin/juju artifacts $env
  
