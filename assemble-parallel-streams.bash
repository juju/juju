#!/bin/bash
set -eu
export JUJU_HOME=$HOME/cloud-city
source $JUJU_HOME/juju-qa.jujuci
set -x
# Prepare for generating in a Jenkins workspace
NEW_STREAMS=$WORKSPACE/new-streams
TESTING=$NEW_STREAMS/testing
AGENT_JSON=$NEW_STREAMS/testing-stanzas
PATH="$HOME/juju-release-tools:$HOME/juju-ci-tools:$PATH"
jujuci.py setup-workspace $WORKSPACE
mkdir -p $TESTING
mkdir -p $AGENT_JSON
cp $HOME/new-streams/testing-stanzas/*.json $AGENT_JSON/
rm $AGENT_JSON/revision-build-$revision_build-paths.json || true

# General-case begins
AGENT_JOBS="build-win-agent build-centos build-binary-precise-amd64 \
   build-binary-trusty-amd64 build-binary-trusty-i386 \
   build-binary-trusty-ppc64el build-binary-vivid-amd64 \
   build-binary-wily-amd64"
WS_JSON=$WORKSPACE/ws-json
WS_AGENTS=$WORKSPACE/agent/revision-build-$revision_build
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
VERSION=$(jujuci.py get-build-vars $revision_build --version)
sstream-query $TESTING/streams/v1/index2.json \
  content_id="com.ubuntu.juju:revision-build-$revision_build:tools" \
  version=$VERSION --output-format="%(sha256)s  %(item_url)s" |sort|uniq > \
  sha256sums
sha256sum -c sha256sums
