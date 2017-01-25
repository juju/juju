#!/bin/bash
set -eu
export JUJU_HOME=${JUJU_HOME:-$HOME/cloud-city}
source $JUJU_HOME/juju-qa.jujuci
set -x
revision_build=$1
WORKSPACE=$2
AGENT_JSON=$3
TESTING=$4
AGENT_JOBS=$5

WS_JSON=$WORKSPACE/ws-json
AGENT_DIRNAME=revision-build-$revision_build
WS_AGENTS=$WORKSPACE/agent/$AGENT_DIRNAME
TESTING_AGENTS=$TESTING/agent/$AGENT_DIRNAME
# set VERSION from buildvars.
source $(s3ci.py get $revision_build build-revision buildvars.bash)
mkdir $WS_JSON
mkdir -p $WS_AGENTS
for job in $AGENT_JOBS; do
  s3ci.py get $revision_build $job '.*\.tgz' $WS_AGENTS
  s3ci.py get $revision_build $job '.*\.json' $WS_JSON
done
set_stream.py $AGENT_JSON/release.json \
  $WS_JSON/release-$revision_build.json $revision_build
mkdir -p $TESTING_AGENTS
cp $WS_AGENTS/* $TESTING_AGENTS
cp $WS_JSON/*.json $AGENT_JSON/
json2streams --juju-format $AGENT_JSON/* $TESTING
sstream-query $TESTING/streams/v1/index2.json \
  content_id="com.ubuntu.juju:revision-build-$revision_build:tools" \
  version=$VERSION --output-format="%(sha256)s  %(item_url)s" |sort|uniq > \
  sha256sums
sha256sum -c sha256sums
