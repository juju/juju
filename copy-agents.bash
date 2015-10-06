#!/bin/bash
set -eux
source_streams=$1
VERSION=$2
agent_path=$3
mkdir -p $agent_path
for sourcepath in $(sstream-query $1/streams/v1/index.json version=$VERSION --output-format="%(item_url)s"); do
    cp $sourcepath $agent_path/$(basename $sourcepath)
done
