set -eu
export JUJU_HOME=$HOME/juju-ci
dump_logs(){
  artifacts_path=$WORKSPACE/artifacts
  mkdir -p $artifacts_path
  log_path=${artifacts_path}/all-machines-${ENV}.log
  if timeout 5m juju --show-log scp -e $ENV -- -o "StrictHostKeyChecking no" -o "UserKnownHostsFile /dev/null" -i $JUJU_HOME/staging-juju-rsa 0:/var/log/juju/all-machines.log $log_path; then
    gzip $log_path
  fi
}
export JUJU_HOME=$HOME/juju-ci
export PACKAGE=$WORKSPACE/new-version.deb
set -x
rm * -rf
artifact=localhost:8080/job/prepare-new-version/lastSuccessfulBuild/artifact
wget -q $artifact/new-version.deb
# Determine BRANCH and REVNO
wget -q $artifact/buildvars.bash
source buildvars.bash
echo "Testing $BRANCH $REVNO on $ENV"
dpkg-deb -x $PACKAGE extracted-bin
export NEW_PATH=$(dirname $(find extracted-bin -name juju)):$PATH
# Try to ensure a clean environment
$SCRIPTS/destroy-environment $ENV
