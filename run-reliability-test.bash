#!/bin/bash
set -eu
: ${SCRIPTS=$(readlink -f $(dirname $0))}
old_stable_juju=$(find $old_stable_juju_dir -name juju)

export JUJU_HOME=$HOME/cloud-city
build_id=${JOB_NAME}-${BUILD_NUMBER}
s3cfg=$JUJU_HOME/juju-qa.s3cfg
s3base=s3://juju-qa-data/industrial-test/${build_id}
if [ "${new_agent_url-}" != "" ]; then
  extra_args="--new-agent-url $new_agent_url"
else
  extra_args=""
fi
set -x
# Delete all files in $WORKSPACE, but no error if empty.
find $WORKSPACE -mindepth 1 -delete
if [ "${revision_build-}" != "" ]; then
  extra_args="$extra_args --agent-stream revision-build-$revision_build"
  new_juju=$($SCRIPTS/s3ci.py get-juju-bin $revision_build $WORKSPACE)
  $SCRIPTS/s3ci.py get $revision_build build-revision buildvars.json $WORKSPACE
  buildvars=$WORKSPACE/buildvars.json
else
  new_juju=$(find $new_juju_dir -name juju)
  buildvars=$new_juju_dir/buildvars.json
fi
if [ "${both_new-}" == "true" ]; then
  export PATH=$(dirname $new_juju):$PATH
fi
logs=$WORKSPACE/logs
mkdir $logs
$SCRIPTS/write_industrial_test_metadata.py $buildvars $environment \
  metadata.json
s3cmd -c $s3cfg put metadata.json $s3base-metadata.json
timeout -sINT -k 10m 1d $SCRIPTS/industrial_test.py $environment $new_juju \
  --old-stable $old_stable_juju $suite $logs --attempts $attempts \
  --json-file results.json $extra_args
s3cmd -c $s3cfg put results.json $s3base-results.json
