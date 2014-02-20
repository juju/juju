set -eu
export JUJU_HOME=$HOME/juju-ci


dump_logs(){
  log_path=${artifacts_path}/all-machines-${ENV}.log
  if [[ $ENV == "local" && -f $JUJU_HOME/local/log/machine-0.log ]]; then
    sudo cp $JUJU_HOME/local/log/*.log $log_path
    sudo chown jenkins:jenkins $log_path
    gzip $log_path
  else
      if timeout 5m juju --show-log scp -e $ENV -- -o "StrictHostKeyChecking no" -o "UserKnownHostsFile /dev/null" -i $JUJU_HOME/staging-juju-rsa 0:/var/log/juju/all-machines.log $log_path; then
        gzip $log_path
      fi
  fi
}


export PACKAGE=$WORKSPACE/new-precise.deb
artifacts_path=$WORKSPACE/artifacts
set -x
rm * -rf
mkdir -p $artifacts_path
touch $artifacts_path/empty
afact='lastSuccessfulBuild/artifact'
wget -q localhost:8080/job/publish-revision/$afact/new-precise.deb
# Determine BRANCH and REVNO
wget -q localhost:8080/job/build-revision/$afact/buildvars.bash
source buildvars.bash
echo "Testing $BRANCH $REVNO on $ENV"
dpkg-deb -x $PACKAGE extracted-bin
export NEW_PATH=$(dirname $(find extracted-bin -name juju)):$PATH
# Try to ensure a clean environment
$SCRIPTS/destroy-environment $ENV
