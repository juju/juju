#!/bin/bash
# Script to run deploy_job against current binaries.
# usage: run-deploy-job.bash {deploy,upgrade} SERIES BASE_ENVIRONMENT TIMEOUT
set -eu
: ${SCRIPTS=$(readlink -f $(dirname $0))}
export SCRIPTS
export USER=jenkins
export JUJU_REPOSITORY=$HOME/repository
export JUJU_HOME=$HOME/cloud-city
export ENV=$3
source $JUJU_HOME/juju-qa.jujuci
set -x
if [ "$1" = "upgrade" ]; then
  extra_args="--upgrade"
elif [ "$1" = "deploy" ]; then
  extra_args=""
else
  echo "Unknown action $1"
  exit 1
fi
series=$2
timeout=$4
shift 4

$SCRIPTS/jujuci.py -v setup-workspace --clean-env $JOB_NAME $WORKSPACE
$SCRIPTS/s3ci.py get-summary 3893 $ENV
JUJU_BIN=$($SCRIPTS/s3ci.py get-juju-bin $revision_build $WORKSPACE)

# Define $VERSION
source $($SCRIPTS/s3ci.py get $revision_build build-revision buildvars.bash)
if [[ $VERSION =~ ^1\.2[1-2].*$ ]]; then
    echo "Setting the default juju to 1.20.11."
    export PATH="$HOME/old-juju/1.20.11/usr/lib/juju-1.20.11/bin:$PATH"
fi
if [[ $VERSION =~ ^1\.23.*$ ]]; then
    echo "Setting the default juju to 1.22.6."
    export PATH="$HOME/old-juju/1.22.6/usr/lib/juju-1.22.6/bin:$PATH"
fi
if [[ $VERSION =~ ^2\..*$ && $extra_args = "--upgrade" ]]; then
    CURRENT_VERSION=$(juju version | cut -d '-' -f 1)
    if [[ ! $CURRENT_VERSION =~ ^2\..*$ ]]; then
        echo "Juju $CURRENT_VERSION does not support upgrade to $VERSION."
        exit 0
    fi
fi

timeout -s INT $timeout $SCRIPTS/deploy_job.py --series $series\
   $ENV $JUJU_BIN $WORKSPACE/artifacts $JOB_NAME $extra_args "$@"
