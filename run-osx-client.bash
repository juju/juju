#!/bin/bash
# This script presumes ~/ci and ~/.juju is setup on the remote machine.
set -eu
SCRIPTS=$(readlink -f $(dirname $0))

usage() {
    echo "usage: $0 user@host revsion_build"
    echo "  user@host: The user and host to ssh to."
    exit 1
}


test $# -eq 2 || usage
USER_AT_HOST="$1"
revision_build="$2"

set -x
$SCRIPTS/jujuci.py get-build-vars --summary $revision_build
if [ -d built ]; then
  rm -R built
fi
mkdir built
s3cmd -c $JUJU_HOME/juju-qa.s3cfg sync \
  s3://juju-qa-data/juju-ci/products/version-$revision_build/build-osx-client/\
  built --exclude '*' --include 'juju-*-osx.tar.gz'
TARFILE=$(find  built -name 'juju-*-osx.tar.gz' | head -1)
echo "Downloaded $TARFILE"

cat > temp-config.yaml <<EOT
install:
  remote:
    - $SCRIPTS/run-osx-client-remote.bash
    - "$TARFILE"
command: [remote/run-osx-client-remote.bash, "remote/$(basename $TARFILE)", "$revision_build"]
EOT
workspace-run temp-config.yaml $USER_AT_HOST
