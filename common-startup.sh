set -eu
: ${SCRIPTS=$(readlink -f $(dirname $0))}
export PATH="$SCRIPTS:$PATH"

export JUJU_HOME=$HOME/cloud-city
if [ "$ENV" = "manual" ]; then
  source $HOME/cloud-city/ec2rc
fi
: ${JUJU_REPOSITORY=$HOME/repository}
export JUJU_REPOSITORY

# Setup workspace and build.
artifacts_path=$WORKSPACE/artifacts
export MACHINES=""
set -x
rm $WORKSPACE/* -rf
mkdir -p $artifacts_path
touch $artifacts_path/empty

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
juju-ci-tools/jujuci.py -d get publish-revision $JUJU_LOCAL_DEB
juju-ci-tools/jujuci.py -d get publish-revision $JUJU_CORE_DEB
dpkg-deb -x $WORKSPACE/$JUJU_CORE_DEB extracted-bin
export NEW_JUJU_BIN=$(readlink -f $(dirname $(find extracted-bin -name juju)))

# Tear down any resources and data last from a previous test.
if [ "$ENV" == "manual" ]; then
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

# Force teardown of generated env names.
jenv=$JUJU_HOME/environments/$JOB_NAME.jenv
if [[ -e $jenv ]]; then
    destroy-environment $JOB_NAME
    if [[ -e $jenv ]]; then
        rm $jenv
    fi
fi

# Force teardown of generated azure env names.
azure_jenvs=$(find $JUJU_HOME/environments -name "$JOB_NAME*.jenv")
for jenv in $azure_jenvs; do
    azure_env=$(basename $jenv .jenv)
    destroy-environment $azure_env
    if [[ -e $jenv ]]; then
        rm $jenv
    fi
done

