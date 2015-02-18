#!/bin/bash
# This script presumes ~/ci and ~/.juju is setup on the remote machine.
set -eu
SCRIPTS=$(readlink -f $(dirname $0))
JUJU_HOME=${JUJU_HOME:-$(dirname $SCRIPTS)/cloud-city}

SSH_OPTIONS="-i $JUJU_HOME/staging-juju-rsa \
    -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"


usage() {
    echo "usage: $0 user@host"
    echo "  user@host: The user and host to ssh to."
    exit 1
}


test $# -eq 1 || usage
USER_AT_HOST="$1"

set -x
$SCRIPTS/jujuci.py get build-revision 'buildvars.bash' ./
source ./buildvars.bash
rev=${REVNO-$(echo $REVISION_ID | head -c8)}
echo "Testing $BRANCH $rev"

ssh $SSH_OPTIONS $USER_AT_HOST <<"EOT"
#!/bin/bash
set -ux
RELEASE_SCRIPTS=$HOME/juju-release-tools
SCRIPTS=$HOME/juju-ci-tools
WORKSPACE=$HOME/workspace
JUJU_HOME=$HOME/cloud-city
source $JUJU_HOME/juju-qa.jujuci
set -e

cd $WORKSPACE
$SCRIPTS/jujuci.py setup-workspace --clean-env testing-osx-client $WORKSPACE
~/Bin/juju destroy-environment --force -y testing-osx-client || true
TARFILE=$($SCRIPTS/jujuci.py get build-osx-client 'juju-*-osx.tar.gz' ./)
echo "Downloaded $TARFILE"
tar -xf ./$TARFILE -C $WORKSPACE

export PATH=$WORKSPACE/juju-bin:$PATH
$SCRIPTS/deploy_stack.py testing-osx-client
EXIT_STATUS=$?
juju destroy-environment -y testing-osx-client || true
exit $EXIT_STATUS
EOT
EXIT_STATUS=$?

exit $EXIT_STATUS
