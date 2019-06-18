#!/bin/bash
# Copyright 2019 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.
current_schema_sha=$(git show HEAD:apiserver/facades/schema.json | shasum -a 1 | awk '{ print $1 }')
tmpfile=$(mktemp /tmp/schema-XXXXX)
make SCHEMA_PATH=$tmpfile rebuild-schema
new_schema_sha=$(cat $tmpfile | shasum -a 1 | awk '{ print $1 }')

if [ $current_schema_sha != $new_schema_sha ]; then
    (>&2 echo "Error: facades schema is not in sync. Run 'make rebuild-schema' and commit source.")
    exit 1
fi
