set -eu
: ${SCRIPTS=$(readlink -f $(dirname $0))}
export PATH="$SCRIPTS:$PATH"
# For most jobs, this is localhost, so provide it.
: ${LOCAL_JENKINS_URL=http://localhost:8080}
export JUJU_HOME=$HOME/cloud-city
if [ "$ENV" = "manual" ]; then
  source $HOME/cloud-city/ec2rc
fi
# Do the deployment for upgrade testing.
: ${JUJU_REPOSITORY=$HOME/repository}

artifacts_path=$WORKSPACE/artifacts
export MACHINES=""
set -x
rm $WORKSPACE/* -rf
mkdir -p $artifacts_path
touch $artifacts_path/empty
afact='lastSuccessfulBuild/artifact'
wget -q $LOCAL_JENKINS_URL/job/publish-revision/$afact/new-precise.deb
# Determine BRANCH and REVNO
wget -q $LOCAL_JENKINS_URL/job/build-revision/$afact/buildvars.bash
source buildvars.bash
echo "Testing $BRANCH $REVNO on $ENV"
dpkg-deb -x $WORKSPACE/new-precise.deb extracted-bin
export NEW_JUJU_BIN=$(readlink -f $(dirname $(find extracted-bin -name juju)))
if [ "$ENV" == "manual" ]; then
  ec2-terminate-job-instances
else
  destroy-environment $ENV
fi
export JUJU_REPOSITORY
jenv=$JUJU_HOME/environments/$ENV.jenv
if [ -e $jenv ]; then rm $jenv; fi
