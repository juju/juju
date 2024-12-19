#!/bin/bash
# Copyright 2019 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.
current_schema_sha=$(git show HEAD:apiserver/facades/schema.json | shasum -a 1 | awk '{ print $1 }')
current_agentschema_sha=$(git show HEAD:apiserver/facades/agent-schema.json | shasum -a 1 | awk '{ print $1 }')
tmpfile=$(mktemp -d /tmp/schema-XXXXX)
make SCHEMA_PATH=$tmpfile rebuild-schema
new_schema_sha=$(cat $tmpfile/schema.json | shasum -a 1 | awk '{ print $1 }')
new_agentschema_sha=$(cat $tmpfile/agent-schema.json | shasum -a 1 | awk '{ print $1 }')

if [ $current_schema_sha != $new_schema_sha ]; then
    (>&2 echo "Error: client facades schema is not in sync. Run 'make rebuild-schema' and commit source.")
    exit 1
fi

if [ $current_agentschema_sha != $new_agentschema_sha ]; then
    (>&2 echo "Error: agent facades schema is not in sync. Run 'make rebuild-schema' and commit source.")
    exit 1
fi
