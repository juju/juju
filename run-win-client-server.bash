#!/bin/bash
set -eu
: ${SCRIPTS=$(readlink -f $(dirname $0))}
export SCRIPTS
export JUJU_HOME=$HOME/cloud-city
source $JUJU_HOME/juju-qa.jujuci
export PATH=$HOME/workspace-runner:$PATH

usage() {
    echo "usage: $0 old-version candidate-version new-to-old agent-arg"
    echo "Example: $0 1.21.1 1.24.3 false \"--agent-stream proposed\" "
    exit 1
}
test $# -eq 4 || usage

old_version="$1"
candidate_version="$2"
new_to_old="$3"
agent_arg="$4"

set -x
# Get revision build from the buildvars file.
buildvars_path=$HOME/candidate/$candidate_version/buildvars.json
revision_build=$(grep revision_build $buildvars_path | grep  -Eo '[0-9]{1,}')
# Windows installer package.
package=juju-setup-$candidate_version.exe

# Get the candidate juju from S3 using the revision build number.
temp_dir=$(mktemp -d)
s3cmd --config $JUJU_HOME/juju-qa.s3cfg sync \
    s3://juju-qa-data/juju-ci/products/version-$revision_build/build-win-client \
    $temp_dir --exclude '*' --include $package
installer=$(find $temp_dir -name $package)
innoextract -e $installer -d $temp_dir
zip -D $temp_dir/juju-$candidate_version-win.zip $temp_dir/app/juju.exe
candidate_juju=$temp_dir/juju-$candidate_version-win.zip

# Get the old juju from S3.
old_package=juju-$old_version-win.zip
old_temp_dir=$(mktemp -d)
s3cmd --config $JUJU_HOME/juju-qa.s3cfg sync \
    s3://juju-qa-data/client-archive/win $old_temp_dir --exclude '*' \
    --include $old_package
old_juju=$(find $old_temp_dir -name $old_package)

if [ "$new_to_old" == "true" ]; then
    server=$candidate_juju
    client=$old_juju
else
    server=$old_juju
    client=$candidate_juju
fi

cat > $old_temp_dir/temp-config.yaml <<EOT
install:
    remote:
        - $SCRIPTS/run-win-client-server-remote.bash
        - "$server"
        - "$client"
command: [remote/run-win-client-server-remote.bash,
          "remote/$(basename $server)", "remote/$(basename $client)",
          "$agent_arg"]
EOT
workspace-run $old_temp_dir/temp-config.yaml Administrator@win-slave.vapour.ws
