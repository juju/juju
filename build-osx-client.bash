#!/bin/bash
# This script presumes ~/ and ~/.juju is setup on the remote machine.
set -eu
SCRIPTS=$(readlink -f $(dirname $0))
JUJU_HOME=${JUJU_HOME:-$(dirname $SCRIPTS)/cloud-city}

usage() {
    echo "usage: $0 user@host revision_build attempt_number"
    echo "  user@host: The user and host to ssh to."
    exit 1
}


test $# -eq 3 || usage
USER_AT_HOST="$1"
revision_build="$2"
attempt_number="$3"


set -x
cat > temp-config.yaml <<EOT
install:
  remote:
    - $SCRIPTS/build-osx-client-remote.bash
command: [remote/build-osx-client-remote.bash]
artifacts:
  client:
    [juju-core_*.tar.gz]
bucket: ws-runner-test
EOT
workspace-run -v --s3-config $JUJU_HOME/juju-qa.s3cfg temp-config.yaml \
  $USER_AT_HOST "juju-ci/products/version-$revision_build/build-osx-client\
  /build-$attempt_number"
