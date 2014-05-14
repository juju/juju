#!/usr/bin/env bash
# Release public tools.
#
# Retrieve the published juju-core debs for a specific release.
# Extract the jujud from the packages.
# Generate the streams data.
# Publish to Canonistack, HP, AWS, and Azure.
#
# This script requires that the user has credentials to upload the tools
# to Canonistack, HP Cloud, AWS, and Azure

set -e


usage() {
	echo usage: $0 RELEASE destination-directory
	exit 1
}


check_deps() {
    has_deps=1
    which lftp || has_deps=0
    which swift || has_deps=0
    which s3cmd || has_deps=0
    test -f ~/.juju/canonistacktoolsrc || has_deps=0
    test -f ~/.juju/hptoolsrc || has_deps=0
    test -f ~/.juju/awstoolsrc || has_deps=0
    test -f ~/.juju/azuretoolsrc || has_deps=0
    if [[ $has_deps == 0 ]]; then
        echo "Install lftp, python-swiftclient, and s3cmd"
        echo "Your ~/.juju dir must contain rc files to publish:"
        echo "  canonistacktoolsrc, hptoolsrc, awstoolsrc, azuretoolsrc"
        exit 2
    fi
}


build_tool_tree() {
    if [[ ! -d $DEST_DEBS ]]; then
        mkdir $DEST_DEBS
    fi
    if [[ ! -d $DEST_TOOLS ]]; then
        mkdir -p $DEST_TOOLS
    fi
    if [[ ! -d $DEST_DIST ]]; then
        mkdir $DEST_DIST
    fi
}


retrieve_released_tools() {
    # Retrieve previously released tools to ensure the metadata continues
    # to work for historic releases.
    source ~/.juju/awstoolsrc
    s3cmd sync s3://juju-dist/tools/releases/ $DEST_TOOLS
}


retrieve_packages() {
    # Retrieve the $RELEASE packages that contain jujud.
    cd $DEST_DEBS
    for archive in $UBUNTU_ARCH $STABLE_ARCH $DEVEL_ARCH; do
        echo "checking $archive for $RELEASE."
        lftp -c mirror -I "juju-core_${RELEASE}*.deb" $archive;
    done
    mv juju-core/*deb ./
    rm -r juju-core
}


get_version() {
    # Defines $version. $version can be different than $RELEASE used to
    # match the packages in the archives.
    control_version=$1
    version=$(echo "$control_version" |
        sed -n 's/^\([0-9]\+\).\([0-9]\+\).\([0-9]\+\)-[0-9].*/\1.\2.\3/p')
    if [ "${version}" == "" ] ; then
	    echo "Invalid version: $control_version"
	    exit 3
    fi
}


get_series() {
    # Defines $series.
    control_version=$1
    pkg_series=$(echo "$control_version" |
        sed -e 's/~juju.//;' \
            -e 's/^.*~\(ubuntu[0-9][0-9]\.[0-9][0-9]\|[a-z]\+\).*/\1/')
    series=${version_names["$pkg_series"]}
    case "${series}" in
	    "precise" | "quantal" | "raring" | "saucy" )
		    ;;
	    *)
		    echo "Invalid series: $control_version"
		    exit 3
		    ;;
    esac
}


get_arch() {
    # Defines $arch.
    control_file=$1
    arch=$(sed -n 's/^Architecture: \([a-z]\+\)/\1/p' $control_file)
    case "${arch}" in
	    "amd64" | "i386" | "armel" | "armhf" | "arm64" | "ppc64el" )
		    ;;
	    *)
		    echo "Invalid arch: $arch"
		    exit 3
		    ;;
    esac
}


archive_tools() {
    # Builds the jujud tgz for each series and arch.
    cd $DESTINATION
    WORK=$(mktemp -d)
    mkdir ${WORK}/juju
    packages=$(ls ${DEST_DEBS}/*.deb)
    for package in $packages; do
        echo "Extracting jujud from ${package}."
        dpkg-deb -e $package ${WORK}/juju
        control_file="${WORK}/juju/control"
        control_version=$(sed -n 's/^Version: \(.*\)/\1/p' $control_file)
        get_version $control_version
        get_series $control_version
        get_arch $control_file
        tool="${DEST_TOOLS}/juju-${version}-${series}-${arch}.tgz"
        echo "Creating $tool."
        dpkg-deb -x $package ${WORK}/juju
        bin_dir="${WORK}/juju/usr/bin"
        lib_dir="${WORK}/juju/usr/lib/juju-${version}/bin"
        if [[ -f "${bin_dir}/jujud" ]]; then
            change_dir=$bin_dir
        elif [[ -f "${lib_dir}/jujud" ]]; then
            change_dir=$lib_dir
        else
            echo "jujud is not in /usr/bin or /usr/lib"
            exit 4
        fi
        tar cvfz $tool -C $change_dir jujud
        echo "Created ${tool}."
        rm -r ${WORK}/juju/*
    done
}


generate_streams() {
    # Create the streams metadata and organised the tree for later publication.
    cd $DESTINATION
    ${GOPATH}/bin/juju sync-tools --all --dev \
        --source=${DESTINATION} --destination=${DEST_DIST}
    # Support old tools location so that deployments can upgrade to new tools.
    cp ${DEST_DIST}/tools/releases/*tgz ${DEST_DIST}/tools
    echo "The tools are in ${DEST_DIST}."
}


publish_to_canonistack() {
    echo "Phase 6.1: Publish to canonistack."
    cd $DESTINATION
    source ~/.juju/canonistacktoolsrc
    ${GOPATH}/bin/juju --show-log \
        sync-tools -e public-tools-canonistack --dev --source=${DEST_DIST}
    # This needed to allow old deployments upgrade.
    cd ${DEST_DIST}
    swift upload juju-dist tools/*.tgz
}


publish_to_hp() {
    echo "Phase 6.2: Publish to HP Cloud."
    cd $DESTINATION
    source ~/.juju/hptoolsrc
    ${GOPATH}/bin/juju --show-log \
        sync-tools -e public-tools-hp --dev --source=${DEST_DIST}
    # Support old tools location so that deployments can upgrade to new tools.
    cd ${DEST_DIST}
    swift upload juju-dist tools/*.tgz
}


publish_to_aws() {
    echo "Phase 6.3: Publish to AWS."
    cd $DESTINATION
    source ~/.juju/awstoolsrc
    s3cmd sync ${DEST_DIST}/tools s3://juju-dist/
}


publish_to_azure() {
    # This command sets the tool name from the local path! The local path for
    # each public file MUST match the destination path :(.
    echo "Phase 6.4: Publish to Azure."
    cd $DESTINATION
    source ~/.juju/azuretoolsrc
    cd ${DEST_DIST}
    public_files=$(find tools -name *.tgz -o -name *.json)
    for public_file in $public_files; do
        echo "Uploading $public_file to Azure West US."
        go run $GOPATH/src/launchpad.net/gwacl/example/storage/run.go \
            -account=${AZURE_ACCOUNT} -container=juju-tools \
            -location="West US" \
            -key=${AZURE_JUJU_TOOLS_KEY} \
            -filename=$public_file \
            addblock
    done
}


# These are the archives that are search for matching releases.
UBUNTU_ARCH="http://archive.ubuntu.com/ubuntu/pool/universe/j/juju-core/"
STABLE_ARCH="http://ppa.launchpad.net/juju/stable/ubuntu/pool/main/j/juju-core/"
DEVEL_ARCH="http://ppa.launchpad.net/juju/devel/ubuntu/pool/main/j/juju-core/"

# Series names found in package versions need to be normalised.
declare -A version_names
version_names+=(["ubuntu12.04"]="precise")
version_names+=(["ubuntu12.10"]="quantal")
version_names+=(["ubuntu13.04"]="raring")
version_names+=(["ubuntu13.10"]="saucy")
version_names+=(["precise"]="precise")
version_names+=(["quantal"]="quantal")
version_names+=(["raring"]="raring")
version_names+=(["saucy"]="saucy")

test $# -eq 2 || usage

RELEASE=$1
DESTINATION=$(cd $2; pwd)
DEST_DEBS="${DESTINATION}/debs"
DEST_TOOLS="${DESTINATION}/tools/releases"
DEST_DIST="${DESTINATION}/juju-dist"

echo "Phase 0: Checking requirements."
check_deps

echo "Phase 1: Building collection and republication tree."
build_tool_tree

echo "Phase 2: Retrieving released tools."
retrieve_released_tools

echo "Phase 3: Retrieving juju-core packages from archives"
retrieve_packages

echo "Phase 4: Extracting jujud from packages and archiving tools."
archive_tools

echo "Phase 5: Generating streams data."
generate_streams

echo "Phase 6: Publishing tools."
publish_to_canonistack
publish_to_hp
publish_to_aws
publish_to_azure
