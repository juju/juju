#!/bin/bash
set -eu
export JUJU_HOME=$HOME/cloud-city
source $JUJU_HOME/juju-qa.jujuci
set -x
PATH="$HOME/juju-release-tools:$HOME/juju-ci-tools:$PATH"
jujuci.py setup-workspace $WORKSPACE
AGENTS=$WORKSPACE/agent/revision-build-$revision_build
mkdir -p $AGENTS
AGENT_JSON=$WORKSPACE/agent-json
mkdir $AGENT_JSON
agent_jobs="build-win-agent build-centos build-binary-precise-amd64 build-binary-trusty-amd64 build-binary-trusty-i386\
   build-binary-trusty-ppc64el build-binary-vivid-amd64  build-binary-wily-amd64"
for job in $agent_jobs; do
  jujuci.py get -b lastBuild $job '*.tgz' $AGENTS
  jujuci.py get -b lastBuild $job '*.json' $AGENT_JSON
done
cp $HOME/new-streams/testing-stanzas/*.json $AGENT_JSON/
rm $AGENT_JSON/revision-build-$revision_build-paths.json || true
set_stream.py $HOME/new-streams/testing-stanzas/release.json $AGENT_JSON/release-$revision_build.json $revision_build
new_streams=$WORKSPACE/new-streams
testing=$new_streams/testing
mkdir -p $testing
cp -r $(dirname $AGENTS) $testing/agent/
json2streams --juju-format $AGENT_JSON/* $testing
VERSION=$(jujuci.py get-build-vars $revision_build --version)
sstream-query $testing/streams/v1/index2.json content_id="com.ubuntu.juju:revision-build-$revision_build:tools" version=$VERSION --output-format="%(sha256)s  %(item_url)s" |sort|uniq > sha256sums
sha256sum -c sha256sums
