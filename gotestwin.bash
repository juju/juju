#!/bin/bash
set -eu
SCRIPTS=$(readlink -f $(dirname $0))
JUJU_HOME=${JUJU_HOME:-$(dirname $SCRIPTS)/cloud-city}

HOST="$1"
REVISION="$2"
PACKAGE=${3:-github.com/juju/juju}

CYG_CI_DIR="/cygdrive/c/Users/Administrator/ci"
CYG_PYTHON_CMD="/cygdrive/c/python27/python"
CI_DIR='\\Users\\Administrator\\ci'
GO_CMD='go.exe'
SSH_OPTIONS="-i $JUJU_HOME/staging-juju-rsa \
    -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"

set -x
DOWNLOADED=$($SCRIPTS/jujuci.py get -b $REVISION build-revision '*' ./)
TARFILE=$(basename $(echo "$DOWNLOADED" | grep -F tar.gz))
$SCRIPTS/jujuci.py get-build-vars --summary --env $HOST $REVISION


cat > temp-config.yaml <<EOT
install:
  ci:
    - ./$TARFILE
    - $SCRIPTS/gotesttarfile.py
    - $SCRIPTS/utility.py
command: [$CYG_PYTHON_CMD, "ci/gotesttarfile.py", -v, -g, "$GO_CMD", "-p",
          "$PACKAGE", "--remove", "ci/$TARFILE"]
EOT

workspace-run -v -i $JUJU_HOME/staging-juju-rsa temp-config.yaml \
    Administrator@$HOST
