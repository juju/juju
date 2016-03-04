#!/bin/bash
set -eux
streams_url=$1
s3_url=$2
revision_build=$3
NEW_VERSION=$4
OLD_VERSION=$5
s3_params="--config $HOME/cloud-city/juju-qa.s3cfg -P"
stream=revision-build-$revision_build
streams_subjson=$streams_url/streams/v1/com.ubuntu.juju-$stream-tools.json

export PATH=$HOME/juju-release-tools:$PATH
content_id="com.ubuntu.juju:$stream:tools"
sstream-query --json $streams_subjson \
  "version~($OLD_VERSION|$NEW_VERSION)" content_id=$content_id \
  release='trusty' arch='amd64'\
  | sed "s/$content_id/com.ubuntu.juju:released:tools/" > released-streams.json
sstream-query --json $streams_subjson "version~($NEW_VERSION)" \
  content_id=$content_id release='trusty' arch='amd64'\
  | sed "s/$content_id/com.ubuntu.juju:devel:tools/" > devel-streams.json
json2streams --juju-format released-streams.json devel-streams.json \
  test-streams
agents=$(sstream-query $streams_subjson \
  "version~($OLD_VERSION|$NEW_VERSION)" content_id=$content_id \
  release='trusty' arch='amd64' --output-format='%(path)s ')
for path in $agents; do
  url=$streams_url/$path
  filename=$(basename $path)
  curl $url -o $filename
  s3cmd put $filename $s3_url/$path $s3_params
done
s3cmd sync test-streams/ $s3_url/ $s3_params
