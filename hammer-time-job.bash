#!/bin/bash
# Required env vars:
# PATH must include juju-ci-tools (for s3ci, jujuci, deploy_job)
# JUJU_HOME must be the path to cloud-city
# JUJU_REPOSITORY must be a path providing dummy-source and dummy-sink
# HAMMER_TIME must be a path to the hammer-time binary.
# base_config is the environment to use as the base config.
# revision_build is the revision build to test.
# action_count is the number of actions the plan should perform
# JOB_NAME and WORKSPACE should be as provided by Jenkins
#
# Optional:
# replay_build_number The number of a previous build to replay.
set -eu
export ARTIFACTS=$WORKSPACE/artifacts
export ARTIFACT_URL=http://juju-ci.vapour.ws/job/$JOB_NAME/\
$replay_build_number/artifact/artifacts/plan.yaml
export MODEL_NAME=$JOB_NAME
export DATA_DIR=$JUJU_HOME/jes-homes/$MODEL_NAME
export PLAN=$ARTIFACTS/plan.yaml
set -x
s3ci.py get-summary $revision_build parallel-lxd
jujuci.py -v setup-workspace $WORKSPACE
if [ -n "${replay_build_number-}" ]; then
  curl --netrc-file $JUJU_HOME/juju-qa-ci.netrc $ARTIFACT_URL -o $PLAN
fi
export JUJU_BIN=$(s3ci.py get-juju-bin $revision_build $WORKSPACE)
set +e
timeout 30m bash <<"EOT"
set -eux
deploy_job.py $base_config $JUJU_BIN $ARTIFACTS $MODEL_NAME \
  --series xenial --agent-stream=revision-build-$revision_build --timeout 600 \
  --keep-env
cd $HOME/hammer-dir/hammer-time
if [ -z "${replay_build_number-}" ]; then
  $HAMMER_TIME run-random $PLAN --juju-data $DATA_DIR --juju-bin $JUJU_BIN \
    --action-count $action_count
else
  $HAMMER_TIME replay $PLAN --juju-data $DATA_DIR --juju-bin $JUJU_BIN
fi
EOT
EXIT_STATUS=$?
set -e
JUJU_DATA=$DATA_DIR juju kill-controller $MODEL_NAME --yes
exit $EXIT_STATUS
