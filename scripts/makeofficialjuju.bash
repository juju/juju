#!/bin/bash

# Copyright 2025 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.
jujud=$(which jujud)
jujudpath=$(dirname "$(which jujud)")
version=$($jujud version)
hash=$(sha256sum $jujud | cut -d " " -f 1)
cat > $jujudpath/jujud-versions.yaml <<EOF
versions:
  - version: $version
    sha256: $hash
EOF

