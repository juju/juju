#!/bin/bash
# Required env vars:
# PATH must include juju-ci-tools (for s3ci, jujuci, deploy_job)
# JUJU_HOME must be the path to cloud-city
# JUJU_REPOSITORY must be a path providing dummy-source and dummy-sink
# HAMMER_TIME must be a path to the hammer-time binary.
# base_config is the environment to use as the base config.
# revision_build is the revision build to test.
# action_count is the number of actions the plan should perform
# series is the OS series to use for machines
# JOB_NAME and WORKSPACE should be as provided by Jenkins
#
# Optional:
# replay_build_number The number of a previous build to replay.
# TIMEOUT The timeout for the operation.  Should be a value acceptable to
#         /usr/bin/timeout, e.g. 35m for 35 minutes.  Default: 30m
set -eu
export ARTIFACTS=$WORKSPACE/artifacts
export MODEL_NAME=$JOB_NAME
export DATA_DIR=$JUJU_HOME/jes-homes/$MODEL_NAME
export S3_CONFIG=$JUJU_HOME/juju-qa.s3cfg
export PLAN=$ARTIFACTS/plan.yaml
export HAMMER_DIR=$(dirname $(dirname $HAMMER_TIME))
: ${TIMEOUT=30m}
set -x
s3ci.py get-summary $revision_build $base_config
source $(s3ci.py get --config $S3_CONFIG $revision_build build-revision buildvars.bash)
if [[ $VERSION =~ ^1\..*$ ]]; then
    echo "$VERSION is not supported for hammer-time."
    exit 0
fi
jujuci.py -v setup-workspace $WORKSPACE
if [ -n "${replay_build_number-}" ]; then
  export ARTIFACT_URL=http://juju-ci.vapour.ws/job/$JOB_NAME/\
$replay_build_number/artifact/artifacts/plan.yaml
  curl -f --netrc-file $JUJU_HOME/juju-qa-ci.netrc $ARTIFACT_URL -o $PLAN
fi
export JUJU_BIN=$(s3ci.py get-juju-bin $revision_build $WORKSPACE)
set +e
timeout $TIMEOUT bash <<"EOT"
set -eux
deploy_job.py $base_config $JUJU_BIN $ARTIFACTS $MODEL_NAME \
  --series $series --agent-stream=revision-build-$revision_build \
  --timeout 600 --keep-env
cd $HAMMER_DIR
if [ -z "${replay_build_number-}" ]; then
  $HAMMER_TIME run-random $PLAN --juju-data $DATA_DIR --juju-bin $JUJU_BIN \
    --action-count $action_count
else
  $HAMMER_TIME replay $PLAN --juju-data $DATA_DIR --juju-bin $JUJU_BIN
fi
EOT
EXIT_STATUS=$?
set -e
JUJU_DATA=$DATA_DIR $JUJU_BIN kill-controller $MODEL_NAME --yes
exit $EXIT_STATUS
