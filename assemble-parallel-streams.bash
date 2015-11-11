#!/bin/bash
set -eu
export JUJU_HOME=$HOME/cloud-city
source $JUJU_HOME/juju-qa.jujuci
set -x
revision_build=$1
WORKSPACE=$2
AGENT_JSON=$3
TESTING=$4
AGENT_JOBS=$5

WS_JSON=$WORKSPACE/ws-json
WS_AGENTS=$WORKSPACE/agent/revision-build-$revision_build
VERSION=$(jujuci.py get-build-vars $revision_build --version)
mkdir $WS_JSON
mkdir -p $WS_AGENTS
for job in $AGENT_JOBS; do
  jujuci.py get -b lastBuild $job '*.tgz' $WS_AGENTS
  jujuci.py get -b lastBuild $job '*.json' $WS_JSON
done
set_stream.py $AGENT_JSON/release.json \
  $WS_JSON/release-$revision_build.json $revision_build
cp -r $(dirname $WS_AGENTS) $TESTING/agent/
cp $WS_JSON/*.json $AGENT_JSON/
json2streams --juju-format $AGENT_JSON/* $TESTING
sstream-query $TESTING/streams/v1/index2.json \
  content_id="com.ubuntu.juju:revision-build-$revision_build:tools" \
  version=$VERSION --output-format="%(sha256)s  %(item_url)s" |sort|uniq > \
  sha256sums
sha256sum -c sha256sums
