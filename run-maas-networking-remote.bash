#!/bin/bash
set -eu
export PATH=$HOME/juju-ci-tools:$PATH
export SCRIPTS=$HOME/juju-ci-tools
export JUJU_REPOSITORY=$HOME/repository
export JUJU_HOME=$HOME/cloud-city
source $HOME/cloud-city/juju-qa.jujuci
WORKSPACE=$(pwd)
TARFILE=$1
source $HOME/.bashrc
set -x

job_name=verify-mass-networking
bundle=cs:~landscape/bundle/landscape-scalable
tar -xf $TARFILE -C $WORKSPACE
mkdir artifacts
chmod +x juju jujud

# run landscape bundle job
timeout -s INT 60m run_deployer.py --debug $bundle parallel-munna-vmaas \
    $WORKSPACE/juju artifacts $job_name --upload-tools \
   --allow-native-deploy \
   --bundle-verification-script $SCRIPTS/verify_landscape_bundle.py \
   --agent-timeout 1800 --workload-timeout 900
