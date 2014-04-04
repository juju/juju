#!/usr/bin/env bash
# Assemble public tools.
#
# Retrieve the published juju-core debs for a specific release.
# Extract the jujud from the packages.
# Generate the streams data.

set -eu


SCRIPT_DIR=$(cd $(dirname "${BASH_SOURCE[0]}") && pwd )


usage() {
    options="[-t TEST_DEBS_DIR]"
    echo "usage: $0 $options RELEASE DESTINATION_DIRECTORY [SIGNING_KEY]"
    echo "  TEST_DEBS_DIR: The optional directory with testing debs."
    echo "  RELEASE: The pattern (version) to match packages in the archives."
    echo "  DESTINATION_DIRECTORY: The directory to assemble the tools in."
    echo "  SIGNING_KEY: When provided, the metadata will be signed."
    exit 1
}


check_deps() {
    echo "Phase 0: Checking requirements."
    has_deps=1
    which lftp || has_deps=0
    which s3cmd || has_deps=0
    test -f $JUJU_DIR/s3cfg || has_deps=0
    test -f $JUJU_DIR/environments.yaml || has_deps=0
    if [[ $has_deps == 0 ]]; then
        echo "Install lftp, s3cmd, then configure s3cmd."
        exit 2
    fi
}


build_tool_tree() {
    echo "Phase 1: Building collection and republication tree."
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
    [[ $PRIVATE == "true" ]] && return 0
    echo "Phase 2: Retrieving released tools."
    # unsupported, stable, devel excludes to make sync fast.
    excludes="--rexclude 'juju-1.15.*' --rexclude 'juju-1.16.[0-5].*'"
    excludes="$excludes --rexclude 'juju-1.17.[0-2].*'"
    s3cmd -c $JUJU_DIR/s3cfg sync $excludes \
        s3://juju-dist/tools/releases/ $DEST_TOOLS/
}


retrieve_packages() {
    # Retrieve the $RELEASE packages that contain jujud,
    # or copy a locally built package.
    [[ $PRIVATE == "true" ]] && return 0
    echo "Phase 3: Retrieving juju-core packages from archives"
    if [[ $IS_TESTING == "true" ]]; then
        for linked_file in $TEST_DEBS_DIR/*.deb; do
            # We need the real file location which includes series and arch.
            deb_file=$(readlink -f $linked_file)
            cp $deb_file $DEST_DEBS
        done
    else
        cd $DEST_DEBS
        for archive in $ALL_ARCHIVES; do
            safe_archive=$(echo "$archive" | sed -e 's,//.*@,//,')
            echo "checking $safe_archive for $RELEASE."
            lftp -c mirror -I "juju-core_${RELEASE}*.deb" $archive;
        done
        if [ -d $DEST_DEBS/juju-core ]; then
            found=$(find $DEST_DEBS/juju-core/ -name "*deb")
            if [[ $found != "" ]]; then
                mv $DEST_DEBS/juju-core/*deb ./
            fi
            rm -r $DEST_DEBS/juju-core
        fi
    fi
}


get_version() {
    # Defines $version. $version can be different than $RELEASE used to
    # match the packages in the archives.
    control_version=$1
    version=$(basename "$control_version" ~juju |
        sed -n 's/^\([0-9]\+\).\([0-9]\+\).\([0-9]\+\)[-+][0-9].*/\1.\2.\3/p')
    if [ "${version}" == "" ] ; then
        echo "Invalid version: $control_version"
        exit 3
    fi
}


get_series() {
    # Defines $series.
    control_version=$1
    pkg_series=$(basename "$control_version" ~juju |
        cut -d '-' -f2 | cut -d '~' -f1 |
        sed -e 's/^[0-9]*ubuntu[0-9]*\.*\([0-9][0-9]\.[0-9][0-9]\).*/\1/')
    if [[ "${!version_names[@]}" =~ ${pkg_series} ]]; then
        series=${version_names["$pkg_series"]}
    else
        # This might be an ubuntu devel series package.
        implied_series=$(echo "$control_version" |
            cut -d '-' -f2- |
            sed -n 's/[0-9]ubuntu[0-9]/DEVEL/p')
        if [[ $implied_series == "DEVEL" ]]; then
            series=$UBUNTU_DEVEL
        else
            echo "Invalid series: $control_version, saw [$pkg_series]"
            echo "${!version_names[@]}"
            exit 3
        fi
    fi
}


get_arch() {
    # Defines $arch.
    control_file=$1
    arch=$(sed -n 's/^Architecture: \([a-z]\+\)/\1/p' $control_file)
    case "${arch}" in
        "amd64" | "i386" | "armel" | "armhf" | "arm64" | "ppc64el" | "powerpc" )
            ;;
        *)
            echo "Invalid arch: $arch"
            arch="UNSUPPORTED"
            ;;
    esac
}


archive_tools() {
    # Builds the jujud tgz for each series and arch.
    [[ $PRIVATE == "true" ]] && return 0
    echo "Phase 4: Extracting jujud from packages and archiving tools."
    cd $DESTINATION
    mkdir ${WORK}/juju
    PACKAGES=$(find ${DEST_DEBS} -name "*.deb")
    for package in $PACKAGES; do
        echo "Extracting jujud from ${package}."
        dpkg-deb -e $package ${WORK}/juju
        control_file="${WORK}/juju/control"
        control_version=$(sed -n 's/^Version: \(.*\)/\1/p' $control_file)
        get_version $control_version
        get_series $control_version
        get_arch $control_file
        if [[ $arch == 'UNSUPPORTED' ]]; then
            echo "Skipping unsupported architecture $package"
            continue
        fi
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
        sane_date=$(ar -tv ${package} |
            grep data.tar |
            cut -d ' ' -f4- |
            sed -e 's,\([^ ]*\) \+\(.*\) \(.*\) \(.*\) .*,\2 \1 \4,')
        touch --date="$sane_date" $change_dir/jujud
        tar cvfz $tool -C $change_dir jujud
        added_tools[${#added_tools[@]}]="$tool"
        echo "Created ${tool}."
        rm -r ${WORK}/juju/*
    done
}


extract_new_juju() {
    # Extract a juju-core that was found in the archives to run metadata.
    # Match by release version and arch, prefer exact series, but fall back
    # to generic ubuntu.
    echo "Phase 5.1: Using juju from a downloaded deb."
    juju_cores=$(find $DEST_DEBS -name "juju-core_${RELEASE}*${ARCH}.deb")
    juju_core=$(echo "$juju_cores" | grep $DISTRIB_RELEASE | head -1)
    if [[ $juju_core == "" ]]; then
        juju_core=$(echo "$juju_cores" | head -1)
    fi
    dpkg-deb -x $juju_core $JUJU_PATH/
    JUJU_EXEC=$(find $JUJU_PATH -name 'juju' | grep bin/juju)
    JUJU_BIN_PATH=$(dirname $JUJU_EXEC)
}



generate_streams() {
    # Create the streams metadata and organised the tree for later publication.
    echo "Phase 5: Generating streams data."
    cd $DESTINATION
    if [[ $RELEASE != "IGNORE" ]]; then
        extract_new_juju
    else
        JUJU_EXEC=$(which juju)
        JUJU_BIN_PATH=""
    fi
    juju_version=$($JUJU_EXEC --version)
    echo "Using juju: $juju_version"
    mkdir -p ${DEST_DIST}/tools/streams/v1
    mkdir -p ${DEST_DIST}/tools/releases
    cp $DEST_TOOLS/*tgz ${DEST_DIST}/tools/releases
    JUJU_HOME=$JUJU_DIR PATH=$JUJU_BIN_PATH:$PATH \
        $JUJU_EXEC metadata generate-tools -d ${DEST_DIST}
    # Support old tools location so that deployments can upgrade to new tools.
    old_tools_glob="${DEST_DIST}/tools/releases/juju-1.16*tgz"
    if [[ $IS_TESTING == "false" && -n $(ls $old_tools_glob) ]]; then
        cp $old_tools_glob ${DEST_DIST}/tools
    fi
    echo "The tools are in ${DEST_DIST}."
}


generate_mirrors() {
    short_now=$(date +%Y%m%d)
    sed -e "s/NOW/$short_now/" ${SCRIPT_DIR}/mirrors.json.template \
        > ${DEST_DIST}/tools/streams/v1/mirrors.json
    long_now=$(date -R)
    sed -e "s/NOW/$long_now/" ${SCRIPT_DIR}/cpc-mirrors.json.template \
        > ${DEST_DIST}/tools/streams/v1/cpc-mirrors.json
}


sign_metadata() {
    echo "Phase 6: Signing metadata with $SIGNING_KEY."
    key_option="--default-key $SIGNING_KEY"
    gpg_options=""
    if [[ -n $SIGNING_PASSPHRASE_FILE ]]; then
        gpg_options="--no-use-agent --no-tty"
        gpg_options="$gpg_options --passphrase-file $SIGNING_PASSPHRASE_FILE"
    fi
    pattern='s/\(\.json\)/.sjson/'
    meta_files=$(ls ${DEST_DIST}/tools/streams/v1/*.json)
    for meta_file in $meta_files; do
        signed_file=$(echo "$meta_file" | sed -e $pattern)
        echo "Creating $signed_file"
        echo "gpg $gpg_options --clearsign $key_option > $signed_file"
        sed -e $pattern $meta_file |
            gpg $gpg_options --clearsign $key_option > $signed_file
        echo "gpg $gpg_options --detach-sign $key_option > $meta_file.gpg"
        cat $meta_file |
            gpg $gpg_options --detach-sign $key_option > $meta_file.gpg
    done
    echo "The signed tools are in ${DEST_DIST}."
}


cleanup() {
    # Remove the debs and testing tools so that they are not reused in
    # future runs of the script.
    if [[ $PACKAGES != "" ]]; then
        rm ${DEST_DEBS}/*.deb
    fi
    if [[ $IS_TESTING == "true" ]]; then
        for tool in "${added_tools[@]}"; do
            rm $tool
        done
    fi
    rm -r $WORK
    rm -r $JUJU_PATH
}


# The location of environments.yaml and rc files.
JUJU_DIR=${JUJU_HOME:-$HOME/.juju}
JUJU_DIR=$(cd $JUJU_DIR; pwd)

# These are the archives that are search for matching releases.
UBUNTU_ARCH="http://archive.ubuntu.com/ubuntu/pool/universe/j/juju-core/"
STABLE_ARCH="http://ppa.launchpad.net/juju/stable/ubuntu/pool/main/j/juju-core/"
DEVEL_ARCH="http://ppa.launchpad.net/juju/devel/ubuntu/pool/main/j/juju-core/"
ARM_ARCH="http://ports.ubuntu.com/pool/universe/j/juju-core/"
ALL_ARCHIVES="$UBUNTU_ARCH $STABLE_ARCH $DEVEL_ARCH $ARM_ARCH"

if [ -f $JUJU_DIR/buildarchrc ]; then
    source $JUJU_DIR/buildarchrc
    ALL_ARCHIVES="$ALL_ARCHIVES $BUILD_STABLE_ARCH $BUILD_DEVEL_ARCH"
fi

# We need to update this constant to ensure ubuntu devel series packages
# are properly identified
UBUNTU_DEVEL="trusty"

# Series names found in package versions need to be normalised.
declare -A version_names
version_names+=(["12.04"]="precise")
version_names+=(["12.10"]="quantal")
version_names+=(["13.04"]="raring")
version_names+=(["13.10"]="saucy")
version_names+=(["14.04"]="trusty")
version_names+=(["precise"]="precise")
version_names+=(["quantal"]="quantal")
version_names+=(["raring"]="raring")
version_names+=(["saucy"]="saucy")
version_names+=(["trusty"]="trusty")

declare -a added_tools
added_tools=()

SIGNING_PASSPHRASE_FILE=${SIGNING_PASSPHRASE_FILE:-}

IS_TESTING="false"
while getopts ":t:" o; do
    case "${o}" in
        t)
            TEST_DEBS_DIR=${OPTARG}
            [[ -d $TEST_DEBS_DIR ]] || usage
            IS_TESTING="true"
            echo "# Assembling test tools from $TEST_DEBS_DIR"
            ;;
        *)
            usage
            ;;
    esac
done
shift $((OPTIND - 1))
test $# -eq 2 || test $# -eq 3 || usage


RELEASE=$1
DESTINATION=$(cd $2; pwd)
DEST_DEBS="${DESTINATION}/debs"
DEST_TOOLS="${DESTINATION}/tools/releases"
DEST_DIST="${DESTINATION}/juju-dist"
if [[ $IS_TESTING == "true" ]]; then
    DEST_DIST="${DESTINATION}/juju-dist-testing"
fi
if [[ -d $DEST_DIST ]]; then
    rm -r $DEST_DIST
fi

SIGNING_KEY=""
PRIVATE="false"
EXTRA=${3:-""}
if [[ $EXTRA == "PRIVATE" ]]; then
    PRIVATE="true"
    echo "Skipping release tools and packages."
else
    SIGNING_KEY=$EXTRA
fi

PACKAGES=""
WORK=$(mktemp -d)
JUJU_PATH=$(mktemp -d)
ARCH=$(dpkg --print-architecture)
source /etc/lsb-release

check_deps
build_tool_tree
retrieve_released_tools
retrieve_packages
archive_tools
generate_streams
generate_mirrors
if [[ $SIGNING_KEY != "" ]]; then
    sign_metadata
fi
cleanup
