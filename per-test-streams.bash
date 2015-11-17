#!/bin/bash
set -eux
s3_url=$1
revision_build=$2
NEW_VERSION=$3
OLD_VERSION=$4
s3_params="--config $HOME/cloud-city/juju-qa.s3cfg -P"

export PATH=$HOME/juju-release-tools:$PATH
content_id="com.ubuntu.juju:revision-build-$revision_build:tools"
TESTING=$HOME/new-streams/parallel
sstream-query --json $TESTING/streams/v1/index2.json \
  "version~($OLD_VERSION|$NEW_VERSION)" content_id=$content_id \
  release='trusty' arch='amd64'\
  | sed "s/$content_id/com.ubuntu.juju:released:tools/" > released-streams.json
sstream-query --json $TESTING/streams/v1/index2.json \
  "version~($NEW_VERSION)" content_id=$content_id \
  release='trusty' arch='amd64'\
  | sed "s/$content_id/com.ubuntu.juju:devel:tools/" > devel-streams.json
json2streams --juju-format released-streams.json devel-streams.json \
  test-streams
agents=$(sstream-query test-streams/streams/v1/index.json \
         --output-format="%(path)s"|sort|uniq)
for agent in $agents; do
  parent=$(dirname $agent)
  if [ $parent = 'agent' ]; then
    source=root
  else
    source=$(basename $TESTING)
  fi
  s3cmd sync $HOME/new-streams/$source/$agent $s3_url/$parent/ $s3_params
done
s3cmd sync test-streams/ $s3_url/ $s3_params
