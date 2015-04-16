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
GO_CMD='\\go\\bin\\go.exe'
SSH_OPTIONS="-i $JUJU_HOME/staging-juju-rsa \
    -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"

set -x
DOWNLOADED=$($SCRIPTS/jujuci.py get -b $REVISION build-revision '*' ./)
TARFILE=$(basename $(echo "$DOWNLOADED" | grep -F tar.gz))
source buildvars.bash
rev=${REVNO-$(echo $REVISION_ID | head -c8)}
echo "Testing $BRANCH $rev"

scp $SSH_OPTIONS ./$TARFILE $SCRIPTS/gotesttarfile.py \
    Administrator@$HOST:$CYG_CI_DIR/
if [ $? -ne 0 ]; then
    exit 1
fi

ssh $SSH_OPTIONS Administrator@$HOST \
    $CYG_PYTHON_CMD $CI_DIR'\\gotesttarfile.py' -v -g $GO_CMD -p $PACKAGE \
    --remove $CI_DIR'\\'$TARFILE
EXIT_STATUS=$?

exit $EXIT_STATUS
