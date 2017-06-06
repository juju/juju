#!/bin/bash
set -eux
revision_build=$1
VERSION=$2
final_path=$3
index=$HOME/streams/juju-dist/testing/tools/streams/v1/index.json
temp_path=revision-build-$revision_build.json
sstream-query --json $index version=$VERSION > $temp_path
set_stream.py $temp_path $final_path $revision_build --update-path
