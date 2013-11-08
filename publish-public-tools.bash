#!/usr/bin/env bash
# Release public tools.
#
# Publish to Canonistack, HP, AWS, and Azure.
# This script requires that the user has credentials to upload the tools
# to Canonistack, HP Cloud, AWS, and Azure

set -e


usage() {
    echo "usage: $0 PURPOSE DIST_DIRECTORY"
    echo "  PURPOSE: 'RELEASE' or  'TESTING'"
    echo "    RELEASE installs tools/ at the top of juju-dist/tools."
    echo "    TESTING installs tools/ at juju-dist/testing/tools."
    echo "  DIST_DIRECTORY: The directory to the assembled tools."
    echo "    This is the juju-dist dir created by assemble-public-tools.bash."
    exit 1
}


check_deps() {
    echo "Phase 0: Checking requirements."
    has_deps=1
    which swift || has_deps=0
    which s3cmd || has_deps=0
    test -f $JUJU_DIR/canonistacktoolsrc || has_deps=0
    test -f $JUJU_DIR/hptoolsrc || has_deps=0
    test -f $JUJU_DIR/s3cfg || has_deps=0
    test -f $JUJU_DIR/azuretoolsrc || has_deps=0
    if [[ $has_deps == 0 ]]; then
        echo "Install python-swiftclient, and s3cmd"
        echo "Your $JUJU_DIR dir must contain rc files to publish:"
        echo "  canonistacktoolsrc, hptoolsrc, awstoolsrc, azuretoolsrc"
        exit 2
    fi
    juju_version=$(juju --version)
    echo "Using installed juju: $juju_version"
}


publish_to_canonistack() {
    echo "Phase 1: Publish to canonistack."
    source $JUJU_DIR/canonistacktoolsrc
    juju --show-log \
        sync-tools -e public-tools-canonistack --dev --source=${JUJU_DIST}
    # This needed to allow old deployments upgrade.
    cd ${JUJU_DIST}
    swift upload --changed juju-dist tools/juju-1.16*.tgz
}


testing_to_canonistack() {
    echo "Phase 1: Testing to canonistack."
    source $JUJU_DIR/canonistacktoolsrc
    cd $JUJU_DIST/tools/releases/
    swift upload --changed juju-dist/testing/tools/releases/ *.tgz
    cd $JUJU_DIST/tools/streams/v1
    swift upload --changed juju-dist/testing/tools/streams/v1/ *.json
}


publish_to_hp() {
    echo "Phase 2: Publish to HP Cloud."
    source $JUJU_DIR/hptoolsrc
    juju --show-log \
        sync-tools -e public-tools-hp --dev --source=${JUJU_DIST}
    # Support old tools location so that deployments can upgrade to new tools.
    cd ${JUJU_DIST}
    swift upload --changed juju-dist tools/juju-1.16*.tgz
}


testing_to_hp() {
    # sync-tools cannot place the tools in a testing location, so swift is
    # used.
    echo "Phase 2: Testing to HP Cloud."
    source $JUJU_DIR/hptoolsrc
    cd $JUJU_DIST/tools/releases/
    swift upload --changed juju-dist/testing/tools/releases/ *.tgz
    cd $JUJU_DIST/tools/streams/v1
    swift upload --changed juju-dist/testing/tools/streams/v1/ *.json
}


publish_to_aws() {
    echo "Phase 3: Publish to AWS."
    s3cmd -c $JUJU_DIR/s3cfg sync ${JUJU_DIST}/tools s3://juju-dist/
}


testing_to_aws() {
    # this is the same as the publishing command except that the
    # destination is juju-dist/testing/
    echo "Phase 3: Testing to AWS."
    s3cmd -c $JUJU_DIR/s3cfg sync ${JUJU_DIST}/tools s3://juju-dist/testing/
}


publish_to_azure() {
    # This command sets the tool name from the local path! The local path for
    # each public file MUST match the destination path :(.
    echo "Phase 4: Publish to Azure."
    source $JUJU_DIR/azuretoolsrc
    cd ${JUJU_DIST}
    public_files=$(find tools -name "*.tgz" -o -name "*.json")
    published_files=$(go run
        $GOPATH/src/launchpad.net/gwacl/example/storage/run.go
        -account=${AZURE_ACCOUNT} -container=juju-tools -location="West US"
        -key=${AZURE_JUJU_TOOLS_KEY} list |
        sed -n 's,.*\(juju.*tgz\),\1,p')
    for public_file in $public_files; do
        [[ $published_files =~ $public_file ]] && continue
        echo "Uploading $public_file to Azure West US."
        go run $GOPATH/src/launchpad.net/gwacl/example/storage/run.go \
            -account=${AZURE_ACCOUNT} -container=juju-tools \
            -location="West US" \
            -key=${AZURE_JUJU_TOOLS_KEY} \
            -filename=$public_file \
            addblock
    done
}


testing_to_azure() {
    # This command is like the publish command expcept tht -container is
    # different.
    echo "Phase 4: Testing to Azure."
    source $JUJU_DIR/azuretoolsrc
    cd ${JUJU_DIST}
    public_files=$(find tools -name "*.tgz" -o -name "*.json")
    published_files=$(go run
        $GOPATH/src/launchpad.net/gwacl/example/storage/run.go
        -account=${AZURE_ACCOUNT} -container=juju-tools -location="West US"
        -key=${AZURE_JUJU_TOOLS_KEY} list |
        sed -n 's,.*\(juju.*tgz\),\1,p')
    for public_file in $public_files; do
        [[ $published_files =~ $public_file ]] && continue
        echo "Uploading $public_file to Azure West US."
        go run $GOPATH/src/launchpad.net/gwacl/example/storage/run.go \
            -account=${AZURE_ACCOUNT} -container=juju-tools/testing \
            -location="West US" \
            -key=${AZURE_JUJU_TOOLS_KEY} \
            -filename=$public_file \
            addblock
    done
}


# The location of environments.yaml and rc files.
JUJU_DIR=${JUJU_HOME:-$HOME/.juju}

test $# -eq 2 || usage

PURPOSE=$1
if [[ $PURPOSE != "RELEASE" && $PURPOSE != "TESTING" ]]; then
    usage
fi

JUJU_DIST=$(cd $2; pwd)
if [[ ! -d $JUJU_DIST/tools/releases && ! -d $JUJU_DIST/tools/streams ]]; then
    usage
fi

check_deps
if [[ $PURPOSE == "RELEASE" ]]; then
    publish_to_canonistack
    publish_to_hp
    publish_to_aws
    publish_to_azure
else
    testing_to_canonistack
    testing_to_hp
    testing_to_aws
    testing_to_azure
fi
