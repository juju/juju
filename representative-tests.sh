#!/bin/bash
set -eu
export USER=jenkins
export SCRIPTS=$HOME/juju-ci-tools
export RELEASE_TOOLS=$HOME/juju-release-tools
export JUJU_REPOSITORY=$HOME/repository
export JUJU_HOME=$HOME/cloud-city
source $JUJU_HOME/juju-qa.jujuci
source $HOME/cloud-city/ec2rc >2 /dev/null
set -x

export PATH=/usr/lib/go-1.6/bin/:$HOME/workspace-runner:$SCRIPTS:$PATH

$SCRIPTS/jujuci.py -v setup-workspace $WORKSPACE

export LANDER_DIR=/var/lib/jenkins/jenkins-github-lander
export GIT_BASE_BRANCH=$base
export GIT_PROPOSED_REPO=$repo
export GIT_REVISION=$ref
export GIT_PULL_REQUEST=$pr
set +e

#We don't allow merges against anything but develop ya hear!
if [[ $base != 'develop' ]]; then
   echo "PR rejected, reporting on PR"
   $LANDER_DIR/bin/lander-merge-result --ini $LANDER_DIR/juju.ini --failure="No PR's or merges are accepted against this branch. Please submit your PR against develop" --pr="${GIT_PULL_REQUEST}"
   exit $EXIT_STATUS
fi

blockers_reason=$($SCRIPTS/check_blockers.py -c $JUJU_HOME/juju-qa-bot-lp-oauth.txt check $GIT_BASE_BRANCH $GIT_PULL_REQUEST)
EXIT_STATUS=$?
set -e
echo $blockers_reason
if [[ $EXIT_STATUS != 0 ]]; then
   echo "Build rejected, reporting on proposal"
   $LANDER_DIR/bin/lander-merge-result --ini $LANDER_DIR/juju.ini --failure="${blockers_reason}" --pr="${GIT_PULL_REQUEST}" --job-name="${JOB_NAME}" --build-number="${BUILD_NUMBER}"
   exit $EXIT_STATUS
fi

echo "Started:" `date`
echo Building $GIT_PROPOSED_REPO revision $GIT_REVISION
echo Build tools revision $(bzr revision-info -d $SCRIPTS)

export GIT_MAIN_REPO="https://github.com/juju/juju.git"

set +e
$RELEASE_TOOLS/make-release-tarball.bash $GIT_BASE_BRANCH $GIT_MAIN_REPO $GIT_REVISION $GIT_PROPOSED_REPO
EXIT_STATUS=$?
set -e
if [[ $EXIT_STATUS != 0 ]]; then
   echo "Build failure, reporting on proposal"
   $LANDER_DIR/bin/lander-merge-result --ini $LANDER_DIR/juju.ini --failure="Generating tarball failed" --pr="${GIT_PULL_REQUEST}" --job-name="${JOB_NAME}" --build-number="${BUILD_NUMBER}"
   exit $EXIT_STATUS
fi
TARFILE=$(find $(pwd) -name juju-core*.tar.gz)
echo Using build tarball $TARFILE
TARFILE_NAME=$(basename "$TARFILE")

export GOPATH=$(dirname $(find $WORKSPACE -type d -name src -regex '.*juju-core[^/]*/src'))
set +e
go install github.com/juju/juju/...
set -e
if [[ $EXIT_STATUS != 0 ]]; then
   echo "Build failure, reporting on proposal"
   $LANDER_DIR/bin/lander-merge-result --ini $LANDER_DIR/juju.ini --failure="Building binary" --pr="${GIT_PULL_REQUEST}" --job-name="${JOB_NAME}" --build-number="${BUILD_NUMBER}"
   exit $EXIT_STATUS
fi
JUJU_BIN=$GOPATH/bin/juju

PRECISE_AMI=$($SCRIPTS/get_ami.py precise amd64)
TRUSTY_AMI=$($SCRIPTS/get_ami.py trusty amd64)
XENIAL_AMI=$($SCRIPTS/get_ami.py xenial amd64)

VERSION=$($JUJU_BIN version | cut -d '-' -f1)
if [[ $VERSION =~ 1\..*  ]]; then
    LXD="echo '1.x does not support lxd'"
    RACE="echo '1.x does not pass race unit tests'"
else
    mkdir artifacts/lxd
    LXD="timeout -s INT 20m $SCRIPTS/deploy_job.py parallel-lxd $JUJU_BIN $WORKSPACE/artifacts/lxd merge-juju-lxd  --series xenial --debug"
    RACE="run-unit-tests m1.xlarge $XENIAL_AMI --force-archive --race --local $TARFILE_NAME --install-deps 'golang-1.6 juju-mongodb distro-info-data ca-certificates bzr git-core mercurial zip golang-1.6-race-detector-runtime'"
    RACE="echo 'Skipping race unit tests.'"
fi
# In case of error, ensure we fail.
EXIT_STATUS=1

set +e
timeout 180m concurrently.py -v -l $WORKSPACE/artifacts \
    trusty="$SCRIPTS/run-unit-tests c3.4xlarge $TRUSTY_AMI --local $TARFILE_NAME --use-tmpfs --force-archive" \
    windows="$SCRIPTS/gotestwin.py developer-win-unit-tester.vapour.ws $TARFILE_NAME github.com/juju/juju/cmd" \
    lxd="$LXD" \
    race="$RACE"
EXIT_STATUS=$?
set -e
if [[ $EXIT_STATUS == 0 ]]; then
    echo "All passed, sending merge"
    $LANDER_DIR/bin/lander-merge-result --ini $LANDER_DIR/juju.ini --pr="${GIT_PULL_REQUEST}" --job-name="${JOB_NAME}" --build-number="${BUILD_NUMBER}" || $LANDER_DIR/bin/lander-merge-result --ini $LANDER_DIR/juju.ini --failure="Merging failed" --pr="${GIT_PULL_REQUEST}" --job-name="${JOB_NAME}" --build-number="${BUILD_NUMBER}"
else
    echo "Test failures, reporting on proposal"
    $LANDER_DIR/bin/lander-merge-result --ini $LANDER_DIR/juju.ini --failure="Tests failed" --pr="${GIT_PULL_REQUEST}" --job-name="${JOB_NAME}" --build-number="${BUILD_NUMBER}"
fi

echo "Finished:" `date`
exit $EXIT_STATUS
