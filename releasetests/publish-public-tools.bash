#!/usr/bin/env bash
# Release public tools.
#
# Publish to Canonistack, AWS, and Azure.
# This script requires that the user has credentials to upload the tools
# to Canonistack, Cloud, AWS, and Azure

set -e

SCRIPT_DIR=$(cd $(dirname "${BASH_SOURCE[0]}") && pwd )

AWS_SITE="http://juju-dist.s3.amazonaws.com"
CAN_SITE="https://swift.canonistack.canonical.com/v1/AUTH_526ad877f3e3464589dc1145dfeaac60/juju-dist"
AZURE_SITE="https://jujutools.blob.core.windows.net/juju-tools"
JOYENT_SITE="https://us-east.manta.joyent.com/cpcjoyentsupport/public/juju-dist"


usage() {
    echo "usage: $0 PURPOSE DIST_DIRECTORY DESTINATIONS"
    echo "  PURPOSE: released, proposed, devel, weekly, or testing"
    echo "    released installs tools/ at the top of juju-dist/tools."
    echo "    proposed installs tools/ at the top of juju-dist/proposed/tools."
    echo "    devel installs tools/ at the top of juju-dist/devel/tools."
    echo "    weekly installs tools/ at juju-dist/weekly/tools."
    echo "    testing installs tools/ at juju-dist/testing/tools."
    echo "  DIST_DIRECTORY: The directory to the assembled tools."
    echo "    This is the juju-dist dir created by assemble-public-tools.bash."
    echo "  DESTINATIONS: cpc or streams"
    echo "    cpc publishes tools to the certified public clouds."
    echo "    streams publishes tools just to streams.canonical.com."
    exit 1
}


verify_stream() {
    [[ -z "$DRY_RUN" ]] || return 0
    local location="$1"
    local stream_path="$2"
    local root=$(echo "$stream_path" | sed -r 's,.*juju-dist/,,')
    echo "Verifying the streams at $location/$root"
    echo "are public and are identical to the source"
    local json_files=$(find $stream_path/streams/v1 -name '*.json')
    for json_file in $json_files; do
        local file_name=$(basename $json_file)
        curl -s $location/$root/streams/v1/$file_name > $WORK/$file_name
        diff -u $json_file $WORK/$file_name
    done
    rm $WORK/*
}


validate_cpcs() {
    verify_stream $AWS_SITE $STREAM_PATH
    verify_stream $CAN_SITE $STREAM_PATH
    verify_stream $AZURE_SITE $STREAM_PATH
    verify_stream $JOYENT_SITE $STREAM_PATH
    rm -r $WORK
    echo "Validated $PURPOSE data for all CPCs."
    exit 0
}


check_deps() {
    echo "Phase 0: Checking requirements."
    has_deps=1
    which swift || has_deps=0
    which s3cmd || has_deps=0
    test -f $JUJU_DIR/canonistacktoolsrc || has_deps=0
    test -f $JUJU_DIR/s3cfg || has_deps=0
    test -f $JUJU_DIR/azuretoolsrc || has_deps=0
    if [[ $has_deps == 0 ]]; then
        echo "Install python-swiftclient, and s3cmd"
        echo "Your $JUJU_DIR dir must contain rc files to publish:"
        echo "  canonistacktoolsrc, s3cfg, azuretoolsrc"
        exit 2
    fi
}


publish_to_aws() {
    [[ $DESTINATIONS == 'cpc' || $DESTINATIONS == 'aws' ]] || return 0
    if [[ $PURPOSE == "released" ]]; then
        local destination="s3://juju-dist/"
    else
        local destination="s3://juju-dist/$PURPOSE/"
    fi
    if [[ ! $PURPOSE =~ ^(devel|proposed)$ ]]; then
        echo "Phase 1: Publishing $PURPOSE to AWS."
        s3cmd -c $JUJU_DIR/s3cfg $DRY_RUN sync --exclude '*mirror*' \
            $STREAM_PATH $destination
        verify_stream $AWS_SITE $STREAM_PATH
    fi
    #
    # New one-tree support.
    #
    if [[ $PURPOSE =~ ^(released|weekly|testing)$ ]]; then
        return
    fi
    echo "Phase 1.1: Publishing $PURPOSE to AWS one-tree."
    s3cmd -c $JUJU_DIR/s3cfg $DRY_RUN sync --exclude '*mirror*' \
        $JUJU_DIST/tools s3://juju-dist/
    verify_stream $AWS_SITE $JUJU_DIST/tools
}


publish_to_canonistack() {
    [[ $DESTINATIONS == 'cpc' || $DESTINATIONS == 'canonistack' ]] || return 0
    [[ "${IGNORE_CANONISTACK-}" == 'true' ]] && return 0
    if [[ $PURPOSE == "released" ]]; then
        local destination="tools"
    else
        local destination="$PURPOSE/tools"
    fi
    echo "Phase 5: Publishing $PURPOSE to canonistack."
    source $JUJU_DIR/canonistacktoolsrc
    if [[ ! $PURPOSE =~ ^(devel|proposed)$ ]]; then
        cd $STREAM_PATH/releases/
        ${SCRIPT_DIR}/swift_sync.py $DRY_RUN -v $destination/releases/ *.tgz
        cd $STREAM_PATH/streams/v1
        ${SCRIPT_DIR}/swift_sync.py $DRY_RUN -v $destination/streams/v1/ {index,com}*
        verify_stream $CAN_SITE $STREAM_PATH
    fi
    #
    # New one-tree support.
    #
    if [[ $PURPOSE =~ ^(released|weekly|testing)$ ]]; then
        return
    fi
    echo "Phase 5.1: Publishing $PURPOSE to canonistack one-tree."
    cd $JUJU_DIST/tools/$PURPOSE/
    ${SCRIPT_DIR}/swift_sync.py $DRY_RUN -v tools/$PURPOSE/ *.tgz
    cd $JUJU_DIST/tools/streams/v1
    ${SCRIPT_DIR}/swift_sync.py $DRY_RUN -v tools/streams/v1/ {index,com}*
    verify_stream $CAN_SITE $JUJU_DIST/tools
}


publish_to_azure() {
    [[ $DESTINATIONS == 'cpc' || $DESTINATIONS == 'azure' ]] || return 0
    echo "Phase 2: Publishing $PURPOSE to Azure."
    source $JUJU_DIR/azuretoolsrc
    if [[ ! $PURPOSE =~ ^(devel|proposed)$ ]]; then
        ${SCRIPT_DIR}/azure_publish_tools.py publish $DRY_RUN $PURPOSE $JUJU_DIST
        verify_stream $AZURE_SITE $STREAM_PATH
    fi
    #
    # New one-tree support.
    #
    if [[ $PURPOSE =~ ^(released|weekly|testing)$ ]]; then
        return
    fi
    ${SCRIPT_DIR}/azure_publish_tools.py publish $DRY_RUN released $JUJU_DIST
    verify_stream $AZURE_SITE $JUJU_DIST/tools
}


publish_to_joyent() {
    [[ $DESTINATIONS == 'cpc' || $DESTINATIONS == 'joyent' ]] || return 0
    [[ "${IGNORE_JOYENT-}" == 'true' ]] && return 0
    if [[ $PURPOSE == "released" ]]; then
        local destination="tools"
    else
        local destination="$PURPOSE/tools"
    fi
    source $JUJU_DIR/joyentrc
    if [[ ! $PURPOSE =~ ^(devel|proposed)$ ]]; then
        echo "Phase 3: Publishing $PURPOSE to Joyent."
        cd $STREAM_PATH/releases/
        ${SCRIPT_DIR}/manta_sync.py $DRY_RUN $destination/releases/ *.tgz
        cd $STREAM_PATH/streams/v1
        ${SCRIPT_DIR}/manta_sync.py $DRY_RUN $destination/streams/v1/ {index,com}*
        verify_stream $JOYENT_SITE $STREAM_PATH
    fi
    #
    # New one-tree support.
    #
    if [[ $PURPOSE =~ ^(released|weekly|testing)$ ]]; then
        return
    fi
    echo "Phase 3.1: Publishing $PURPOSE to Joyent one-tree."
    cd $JUJU_DIST/tools/$PURPOSE/
    ${SCRIPT_DIR}/manta_sync.py $DRY_RUN tools/$PURPOSE/ *.tgz
    cd $JUJU_DIST/tools/streams/v1
    ${SCRIPT_DIR}/manta_sync.py $DRY_RUN tools/streams/v1/ {index,com}*
    verify_stream $JOYENT_SITE $JUJU_DIST/tools
}


publish_to_streams() {
    [[ $DESTINATIONS == 'streams' ]] ||  return 0
    echo "Phase 6: Publishing $PURPOSE to streams.canonical.com."
    source $JUJU_DIR/streamsrc
    destination=$STREAMS_OFFICIAL_DEST
    rsync $DRY_RUN -avzh $JUJU_DIST/ $destination
}


# The location of environments.yaml and rc files.
JUJU_DIR=${JUJU_HOME:-$HOME/.juju}

DRY_RUN=""
ONLY_VALIDATE="false"
if  [[ "$1" == "--dry-run" ]]; then
    DRY_RUN="--dry-run"
    echo "No changes will be made."
    shift
elif  [[ "$1" == "--validate" ]]; then
    ONLY_VALIDATE="true"
    echo "Validating streams, no changes will be made."
    shift
fi

test $# -eq 3 || usage

PURPOSE=$1
if [[ ! $PURPOSE =~ ^(released|proposed|devel|weekly|testing)$ ]]; then
    echo "Invalid PURPOSE."
    usage
fi

JUJU_DIST=$(cd $2; pwd)
if [[ $PURPOSE == "released" ]]; then
    STREAM_PATH="$JUJU_DIST/tools"
else
    STREAM_PATH="$JUJU_DIST/$PURPOSE/tools"
fi
if [[ ! -d $STREAM_PATH/releases && ! -d $STREAM_PATH/streams ]]; then
    echo "Invalid JUJU-DIST: $STREAM_PATH"
    usage
fi

DESTINATIONS=$3
if [[ ! $DESTINATIONS =~ ^(aws|azure|canonistack|joyent|cpc|streams)$ ]]; then
    echo "Invalid DESTINATIONS."
    usage
fi


check_deps
WORK=$(mktemp -d)
if [[ $ONLY_VALIDATE == 'true' ]]; then
    validate_cpcs
fi
publish_to_aws
publish_to_azure
publish_to_joyent
publish_to_canonistack
publish_to_streams
rm -r $WORK
echo "Published $PURPOSE data to $DESTINATIONS."
