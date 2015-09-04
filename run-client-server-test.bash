#!/bin/bash
set -eu
SCRIPTS=$(readlink -f $(dirname $0))
export PATH=$HOME/workspace-runner:$PATH

usage() {
    echo "usage: $0 old-version candidate-version new-to-old client-os revision-build log-dir"
    exit 1
}
test $# -eq 6 || usage
old_version="$1"
candidate_version="$2"
new_to_old="$3"
client_os="$4"
revision_build="$5"
log_dir="$6"

set -x
if [[ "$client_os" == "ubuntu" ]]; then
    old_juju=$(find $HOME/old-juju/$old_version -name juju)
    candidate_juju=$(find $HOME/candidate/$candidate_version -name juju)
elif [[ "$client_os" == "osx" ]]; then
    package=juju-$candidate_version-osx.tar.gz
    build_dir="build-osx-client"
    old_package=juju-$old_version-osx.tar.gz
    archive_dir="osx"
    user_at_host="jenkins@osx-slave.vapour.ws"
    remote_script="run-client-server-test-remote.bash"
elif [[ "$client_os" == "windows" ]]; then
    package=juju-setup-$candidate_version.exe
    build_dir="build-win-client"
    old_package=juju-$old_version-win.zip
    archive_dir="win"
    user_at_host="Administrator@win-slave.vapour.ws"
    remote_script="run-win-client-server-remote.bash"
else
    echo "Unkown client OS."
    exit 1
fi

# Get OS X and Windows Juju from S3.
if [[ "$client_os" == "osx" ]] || [[ "$client_os" == "windows" ]]; then
    temp_dir=$(mktemp -d)
    s3cmd --config $JUJU_HOME/juju-qa.s3cfg sync \
        s3://juju-qa-data/juju-ci/products/version-$revision_build/$build_dir \
        $temp_dir --exclude '*' --include $package
    candidate_juju=$(find $temp_dir -name $package)
    if [ "$client_os" == "windows" ]; then
        # Extract Windows exe file and compress it.
        innoextract -e $candidate_juju -d $temp_dir
        zip -D $temp_dir/juju-$candidate_version-win.zip $temp_dir/app/juju.exe
        candidate_juju=$temp_dir/juju-$candidate_version-win.zip
    fi
    # Get the old juju from S3.
    old_temp_dir=$(mktemp -d)
    s3cmd --config $JUJU_HOME/juju-qa.s3cfg sync \
        s3://juju-qa-data/client-archive/$archive_dir $old_temp_dir --exclude '*' \
        --include $old_package
    old_juju=$(find $old_temp_dir -name $old_package)
fi

if [[ "$new_to_old" == "true" ]]; then
    server=$candidate_juju
    client=$old_juju
    echo "Using weekly streams for unreleased version"
    agent_arg="--agent-url http://juju-dist.s3.amazonaws.com/weekly/tools"
else
    server=$old_juju
    client=$candidate_juju
    echo "Using official proposed (or released) streams"
    agent_arg="--agent-stream proposed"
fi

run_remote_script() {
    cat > temp-config.yaml <<EOT
install:
    remote:
        - $SCRIPTS/$remote_script
    client:
        - $client
    server:
        - $server
command: [remote/$remote_script,
          "server/$(basename $server)", "client/$(basename $client)",
          "$agent_arg"]
EOT
    workspace-run temp-config.yaml $user_at_host
}

set +e
for i in `seq 1 2`; do
    if [[ "$client_os" == "ubuntu" ]]; then
        $SCRIPTS/assess_heterogeneous_control.py $server $client test-reliability-aws $JOB_NAME $log_dir $agent_arg
    else
        run_remote_script
    fi
    if [[ $? == 0 ]]; then
        break
    fi
    rm -r $log_dir/*
done
