set -eu
if [ "$ENV" = "manual" ]; then
  export JUJU_HOME=$WORKSPACE/manual-provider-home
  source $HOME/juju-ci/ec2rc
else
  export JUJU_HOME=$HOME/juju-ci
fi

dump_logs(){
  log_path=${artifacts_path}/all-machines-${ENV}.log
  if [[ $ENV == "local" && -f $JUJU_HOME/local/log/machine-0.log ]]; then
    sudo cp $JUJU_HOME/local/log/*.log $artifacts_path/
    sudo chown jenkins:jenkins $artifacts_path/*.log
    for log in $artifacts_path/*.log; do
        gzip $log
    done
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
JUJU_BIN=$(readlink -f $(dirname $(find extracted-bin -name juju)))
export NEW_PATH=$JUJU_BIN:$PATH
if [ "$ENV" != "manual" ]; then
  $SCRIPTS/destroy-environment $ENV
fi
