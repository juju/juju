#!/bin/bash
# Copyright 2019 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.
function schema_sha {
    echo `cat ./apiserver/facades/schema.json | sha256sum | awk '{ print $1 }'`
}

current_schema_sha=$(schema_sha)
make rebuild-schema
new_schema_sha=$(schema_sha)
if [ $current_schema_sha != $new_schema_sha ]; then
    (>&2 echo "Error: facades schema is not in sync. Run `make rebuild-schema` and commit source.")
fi