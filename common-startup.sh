set -eu
# For most jobs, this is localhost, so provide it.
: ${LOCAL_JENKINS_URL=http://localhost:8080}
if [ "$ENV" = "manual" ]; then
  export JUJU_HOME=$WORKSPACE/manual-provider-home
  source $HOME/cloud-city/ec2rc
else
  export JUJU_HOME=$HOME/cloud-city
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
prepare_manual(){
    export INSTANCE_TYPE=m1.large
    export AMI_IMAGE=ami-bd6d40d4
    machine_0_id=$($SCRIPTS/ec2-run-instance-get-id -g manual-juju-test)
    machine_1_id=$($SCRIPTS/ec2-run-instance-get-id -g manual-juju-test)
    machine_2_id=$($SCRIPTS/ec2-run-instance-get-id -g manual-juju-test)
    $SCRIPTS/ec2-tag-job-instances $machine_0_id $machine_1_id $machine_2_id
    machine_0_name=$($SCRIPTS/ec2-get-name $machine_0_id)
    machine_1_name=$($SCRIPTS/ec2-get-name $machine_1_id)
    machine_2_name=$($SCRIPTS/ec2-get-name $machine_2_id)
    export BOOTSTRAP_HOST=$machine_0_name

    if [ ! -d $JUJU_HOME ]; then
        mkdir $JUJU_HOME
    fi
    rm -f $JUJU_HOME/environments/manual.jenv

    # Write new environments.yaml
    cat  > $JUJU_HOME/environments.yaml <<EOT
environments:
  manual:
    type: manual
    bootstrap-user: ubuntu
    tools-metadata-url: http://juju-dist.s3.amazonaws.com/testing/tools
EOT
    for machine in $machine_0_name $machine_1_name $machine_2_name; do
      $SCRIPTS/wait-for-port $machine 22
    done
    DEPLOY_ARGS="--machine ssh:$machine_1_name --machine ssh:$machine_2_name"
    export MACHINES="$machine_1_name $machine_2_name"
}

export PACKAGE=$WORKSPACE/new-precise.deb
artifacts_path=$WORKSPACE/artifacts
export MACHINES=""
set -x
rm * -rf
mkdir -p $artifacts_path
touch $artifacts_path/empty
afact='lastSuccessfulBuild/artifact'
wget -q $LOCAL_JENKINS_URL/job/publish-revision/$afact/new-precise.deb
# Determine BRANCH and REVNO
wget -q $LOCAL_JENKINS_URL/job/build-revision/$afact/buildvars.bash
source buildvars.bash
echo "Testing $BRANCH $REVNO on $ENV"
dpkg-deb -x $PACKAGE extracted-bin
JUJU_BIN=$(readlink -f $(dirname $(find extracted-bin -name juju)))
export NEW_PATH=$JUJU_BIN:$PATH
if [ "$ENV" == "manual" ]; then
  $SCRIPTS/ec2-terminate-job-instances
else
  $SCRIPTS/destroy-environment $ENV
fi
jenv=$JUJU_HOME/environments/$ENV.jenv
if [ -e $jenv ]; then rm $jenv; fi
