#!/bin/bash
set -eu
: ${SCRIPTS=$(readlink -f $(dirname $0))}
export SCRIPTS
export USER=jenkins
export JUJU_REPOSITORY=$HOME/repository
export JUJU_HOME=$HOME/cloud-city
export ENV=$1
source $JUJU_HOME/juju-qa.jujuci
set -x
if [ "$2" = "upgrade" ]; then
  extra_args="--upgrade"
elif [ "$2" = "deploy" ]; then
  extra_args=""
else
  echo "Unknown action $2"
  exit 1
fi
$SCRIPTS/jujuci.py -v setup-workspace --clean-env $JOB_NAME $WORKSPACE
JUJU_BIN=$(dirname $($SCRIPTS/jujuci.py get-juju-bin))
$SCRIPTS/jujuci.py get build-revision buildvars.bash ./
# Avoid polluting environment when printing branch/revision
bash -eu <<'EOT'
source buildvars.bash
rev=${REVNO-$(echo $REVISION_ID | head -c7)}
echo "Testing $BRANCH $rev on $ENV"
EOT
timeout -s INT $3 $SCRIPTS/deploy_job.py --new-juju-bin $JUJU_BIN\
  --series trusty $ENV $WORKSPACE/artifacts $JOB_NAME $extra_args
