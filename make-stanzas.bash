#!/bin/bash
set -eux
revision_build=$1
VERSION=$2
final_path=$3
sstream-query $HOME/streams/juju-dist/testing/tools/streams/v1/index.json version=$VERSION > revision-build-$revision_build.repr
ssquery_json.py revision-build-$revision_build.repr revision-build-$revision_build.json
set_stream.py revision-build-$revision_build.json $final_path $revision_build --update-path
