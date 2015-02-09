set -eu
: ${SCRIPTS=$(readlink -f $(dirname $0))}
export PATH="$SCRIPTS:$PATH"

export JUJU_HOME=${JUJU_HOME:-$HOME/cloud-city}
: ${JUJU_REPOSITORY=$HOME/repository}
export JUJU_REPOSITORY

export MACHINES=""
set -x

# Determine BRANCH, REVNO, VERSION, and PACKAGES under test.
RELEASE=$(lsb_release -sr)
ARCH=$(dpkg --print-architecture)
if [[ -n ${revision_build:-} ]]; then
    $SCRIPTS/jujuci.py get build-revision buildvars.bash ./
    source buildvars.bash
    PACKAGES_JOB="publish-revision"
    JUJU_LOCAL_DEB="juju-local_$VERSION-0ubuntu1~$RELEASE.1~juju1_all.deb"
    JUJU_CORE_DEB="juju-core_$VERSION-0ubuntu1~$RELEASE.1~juju1_$ARCH.deb"
    rev=${REVNO-$(echo $REVISION_ID | head -c8)}
    echo "Testing $BRANCH $rev on $ENV"
elif [[ -n ${VERSION:-} ]]; then
    PACKAGES_JOB="certify-ubuntu-packages"
    JUJU_LOCAL_DEB="juju-local_$VERSION.$RELEASE.1_all.deb"
    JUJU_CORE_DEB="juju-core_$VERSION.$RELEASE.1_$ARCH.deb"
    echo "Testing $VERSION on $ENV"
else
    echo "Job didn't define revision_build or VERSION"
    exit 1
fi

# Provide the juju-core and juju-local packages to the test
$SCRIPTS/jujuci.py get publish-revision $JUJU_LOCAL_DEB
$SCRIPTS/jujuci.py get publish-revision $JUJU_CORE_DEB
dpkg-deb -x ./$JUJU_CORE_DEB extracted-bin
export NEW_JUJU_BIN=$(readlink -f $(dirname $(find extracted-bin -name juju)))

set +x
# Tear down any resources and data last from a previous test.
if [ "$ENV" == "manual" ]; then
    source $HOME/cloud-city/ec2rc
    ec2-terminate-job-instances
else
    jenv=$JUJU_HOME/environments/$ENV.jenv
    if [[ -e $jenv ]]; then
        destroy-environment $ENV
        if [[ -e $jenv ]]; then
            rm $jenv
        fi
    fi
fi
