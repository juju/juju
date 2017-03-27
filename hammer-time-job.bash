#!/bin/bash
set -eu
export ARTIFACTS=$WORKSPACE/artifacts
export ARTIFACT_URL=http://juju-ci.vapour.ws/job/hammer-time/\
$replay_build_number/artifact/artifacts/plan.yaml
export MODEL_NAME=$JOB_NAME
export DATA_DIR=$JUJU_HOME/jes-homes/$MODEL_NAME
export PLAN=$ARTIFACTS/plan.yaml
source $JUJU_HOME/juju-qa.jujuci
set -x
s3ci.py get-summary $revision_build parallel-lxd
jujuci.py -v setup-workspace $WORKSPACE
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
  set +x
  curl $ARTIFACT_URL -u $JENKINS_USER:$JENKINS_PASSWORD -o $PLAN
  set -x
  $HAMMER_TIME replay $PLAN --juju-data $DATA_DIR --juju-bin $JUJU_BIN
fi
EOT
EXIT_STATUS=$?
set -e
JUJU_DATA=$DATA_DIR juju kill-controller $MODEL_NAME --yes
exit $EXIT_STATUS
