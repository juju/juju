new_juju=$(find $new_juju_dir -name juju)
export JUJU_HOME=$HOME/cloud-city
$HOME/juju-ci-tools/write_industrial_test_metadata.py $new_juju_dir/buildvars.json $environment metadata.json
build_id=${JOB_NAME}-${BUILD_NUMBER}
s3cmd -c ~/cloud-city/juju-qa.s3cfg put metadata.json s3://juju-qa-data/industrial-test/${build_id}-metadata.json
if [ "$new_agent_url" != "" ]; then
  extra_args="--new-agent-url $new_agent_url"
else
  extra_args=""
fi
$HOME/juju-ci-tools/industrial_test.py $environment $new_juju $suite --attempts $attempts \
    --json-file results.json $extra_args
s3cmd -c ~/cloud-city/juju-qa.s3cfg put results.json s3://juju-qa-data/industrial-test/${build_id}-results.json
