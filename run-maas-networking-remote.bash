#!/bin/bash
set -eu
export PATH=$HOME/juju-ci-tools:$PATH
WORKSPACE=$(pwd)
TARFILE=$1
source $HOME/.bashrc
set -x

env=parallel-lxd
job_name=verify-mass-networking
bundle=cs:~landscape/bundle/landscape-scalable
tar -xf $TARFILE -C $WORKSPACE
mkdir artifacts

# run basic LXD deploy
timeout -s INT 20m deploy_job.py --series xenial \
   parallel-lxd $WORKSPACE/juju-bin/juju artifacts $job_name --upload-tools

# run landscape bundle job
timeout -s INT 60m run_deployer.py --debug $bundle parallel-munna-vmaas \
    $WORKSPACE/juju-bin/juju artifacts $job_name --upload-tools \
   --allow-native-deploy \
   --bundle-verification-script $SCRIPTS/verify_landscape_bundle.py \
   --agent-timeout 1800 --workload-timeout 900
