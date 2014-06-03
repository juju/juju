set -eu
: ${SCRIPTS=$(readlink -f $(dirname $0))}
export PATH="$SCRIPTS:$PATH"

# For most jobs, this is localhost, so provide it.
: ${LOCAL_JENKINS_URL=http://localhost:8080}
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

# Determine BRANCH, REVNO, and VERSION
afact='lastSuccessfulBuild/artifact'
wget -q $LOCAL_JENKINS_URL/job/build-revision/$afact/buildvars.bash
source buildvars.bash
set +u
if [[ -n "$REVNO" ]]
    rev=$REVNO
else
    rev=$REVISION_ID
fi
set -u
echo "Testing $BRANCH $rev on $ENV"

# Provide the juju-core and juju-local packages to the test
RELEASE=$(lsb_release -sr)
ARCH=$(dpkg --print-architecture)
juju_local_deb="juju-local_$VERSION-0ubuntu1~$RELEASE.1~juju1_all.deb"
juju_core_deb="juju-core_$VERSION-0ubuntu1~$RELEASE.1~juju1_$ARCH.deb"
wget -q $LOCAL_JENKINS_URL/job/publish-revision/$afact/$juju_local_deb
wget -q $LOCAL_JENKINS_URL/job/publish-revision/$afact/$juju_core_deb
dpkg-deb -x $WORKSPACE/$juju_core_deb extracted-bin
export NEW_JUJU_BIN=$(readlink -f $(dirname $(find extracted-bin -name juju)))

# Tear down any resources and data last from a previous test.
if [ "$ENV" == "manual" ]; then
    ec2-terminate-job-instances
else
    jenv=$JUJU_HOME/environments/$ENV.jenv
    if [[ -e $jenv || $JOB_NAME == "test-release-azure3" ]]; then
        destroy-environment $ENV
        if [[ -e $jenv ]]; then
            rm $jenv
        fi
    fi
fi

# Force teardown of generated env names.
jenv=$JUJU_HOME/environments/$JOB_NAME.jenv
if [[ -e $jenv || $JOB_NAME == "azure-upgrade-precise-amd64" ]]; then
    destroy-environment $JOB_NAME
    if [[ -e $jenv ]]; then
        rm $jenv
    fi
fi

