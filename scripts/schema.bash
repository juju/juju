#!/bin/bash
# Copyright 2019 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.
current_schema_sha=$(git show HEAD:apiserver/facades/schema.json | sha256sum | awk '{ print $1 }')
tmpfile=$(mktemp /tmp/schema-XXXXX.json)
make SCHEMA_PATH=$tmpfile rebuild-schema
new_schema_sha=$(cat $tmpfile | sha256sum | awk '{ print $1 }')

if [ $current_schema_sha != $new_schema_sha ]; then
    (>&2 echo "Error: facades schema is not in sync. Run 'make rebuild-schema' and commit source.")
fi
