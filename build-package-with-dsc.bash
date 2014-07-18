#!/bin/bash

# IMPORTANT: The ssh options are always remapped to $@ which preserves
# their tokenisation.


usage() {
    echo "usage: $0 [<instance-type> <ami> | <[user@]host> '<ssh-options>'] dsc"
    echo "  Build binary packages on an ec2 instance or a remote host."
    echo ""
    echo "  instance-type: the ec2 instance type, like m1.large."
    echo "  ami: The ec2 image, it defines the series and arch to build."
    echo "  user@host: host to build on. 'user@' is optional."
    echo "      Use localhost to build locally."
    echo "  ssh-options: a string of all the options to establish a connection."
    echo "      Like '-o \"ProxyCommand ssh user@10.10.10.10 nc -q0 %h %p\"'."
    echo "  dsc: The path the the dsc file."
}


create_instance() {
    echo "Creating an instance to build juju on."
    if [[ -z $AWS_SECRET_KEY ]]
    then
        echo "Source your ec2 credentials to create an instance."
        exit 1
    fi
    export JOB_NAME=${JOB_NAME:-$USER-build-package}
    INSTANCE_ID=$($SCRIPTS/ec2-run-instance-get-id)
    $SCRIPTS/ec2-tag-job-instances $INSTANCE_ID
    set +x
    echo Starting instance $INSTANCE_ID
    INSTANCE_NAME=$($SCRIPTS/ec2-get-name $INSTANCE_ID)
    echo Instance has ip $INSTANCE_NAME
    sleep 30
    $SCRIPTS/wait-for-port $INSTANCE_NAME 22
    # Pause, do not start until cloud-init has updated the apt sources.
    ssh "$@" $REMOTE_USER@$INSTANCE_NAME  <<EOT
    set -eux
    for attempt in \$(seq 10); do
        if grep -r '^deb .* universe$' /etc/apt/sources.list > /dev/null
        then
            break
        elif [ "\$attempt" == "10" ]; then
            echo "Universe is not available to install packages from."
            exit 1
        fi
        sleep 5m
    done
EOT
}


install_build_deps() {
    echo "Installing build deps."
    remote_compiler=$(ssh "$@" $REMOTE_USER@$INSTANCE_NAME  <<EOT
        if [[ \$(uname -p) =~  .*86|armel|armhf.* ]]; then
            echo "golang"
        else
            echo "gccgo-4.9 gccgo-go"
        fi
EOT
    )
    juju_compiler=$(echo "$remote_compiler" | tail -1)
    DEPS="build-essential fakeroot dpkg-dev debhelper bash-completion $juju_compiler"

    DEP_SCRIPT=$(cat <<EOT
        sudo sed s/ec2.archive.ubuntu.com/archive.ubuntu.com/ /etc/apt/sources.list -i
        needs_ppa=\$(lsb_release -sc | sed -r 's,precise,true,')
        if [[ \$needs_ppa == 'true' ]]; then
            sudo apt-add-repository -y ppa:juju/experimental;
        fi
        sudo apt-get update;
        sudo apt-get install -y $DEPS;
EOT
    )

    if [[ $INSTANCE_NAME = "localhost" ]]; then
        eval "$DEP_SCRIPT"
    else
        ssh "$@" $REMOTE_USER@$INSTANCE_NAME "$DEP_SCRIPT"
    fi
}


upload_source_package_files() {
    echo "Uploading source package files."
    source_dir=$(dirname $DSC)
    # The source files are listed between the Checksums-Sha256 and Files
    # sections in the dsc. The spaces are replaces with ":" to make data easy
    # to cut.
    source_file_data=$(
        sed -n '/^Checksums-Sha256/,/^Files/!d; /^ /!d; s,^ ,,; s, ,:,gp' $DSC)
    source_files="$DSC"
    echo "Found these files in $source_dir to upload:"
    for data in $source_file_data
    do
        file_sha256=$(echo "$data" | cut -d : -f 1)
        file_size=$(echo "$data" | cut -d : -f 2)
        file_name=$(echo "$data" | cut -d : -f 3)
        echo "$file_name"
        source_files="$source_files $source_dir/$file_name"
    done
    ssh "$@" $REMOTE_USER@$INSTANCE_NAME <<EOT
        if [[ ! -d $THERE/juju-build/ ]]; then
            mkdir $THERE/juju-build
        fi
EOT
    scp "$@" $source_files $REMOTE_USER@$INSTANCE_NAME:$THERE/juju-build/
}


build_binary_packages() {
    echo "Building binary packages"
    juju_version=$(basename $DSC .dsc)
    version=$(echo $juju_version | cut -d _ -f2 | sed -r 's,-0ubuntu.*$,,;')
    ssh "$@" $REMOTE_USER@$INSTANCE_NAME <<EOT
        set -eux
        cd $THERE/juju-build/
        go version || gccgo -v
        dpkg-source -x $juju_version.dsc
        cd juju-core-$version
        if [[ -n \$(pidof go) ]]; then
            # Go is either building or testing; do not clean its procs in /tmp.
            build_opts="-nc"
        else
            build_opts=""
        fi
        dpkg-buildpackage -us -uc \$build_opts
EOT
}


retrieve_binary_packages() {
    echo "Retrieving built packages."
    scp "$@" $REMOTE_USER@$INSTANCE_NAME:$THERE/juju-build/*.deb ./
    EXIT_STATUS=$?
}


cleanup() {
    echo "Cleaning up remote"
    if [[ $IS_EPHEMERAL == "true" ]]; then
        $SCRIPTS/ec2-terminate-job-instances
    else
        ssh "$@" $REMOTE_USER@$INSTANCE_NAME \
            "test -d $THERE/juju-build && rm -r $THERE/juju-build"
    fi
}


if [[ $1 = "--help" ]]; then
    set +x
    usage
    exit
fi


# Do not set -e because this script must cleanup its resources.
set -ux
ssh_options='-o "StrictHostKeyChecking no" -o "UserKnownHostsFile /dev/null"'
EXIT_STATUS=1
SCRIPTS=${SCRIPTS:-$(dirname $(dirname $0))/juju-ci-tools}
DSC=$(readlink -f $3)

if [[ $1 = "localhost" ]]; then
    # Use the local user and working directory to build juju.
    IS_EPHEMERAL="false"
    THERE=$(pwd)
    REMOTE_USER=$USER
    INSTANCE_NAME=$1
    # Remap ssh option strings to $@ to preserve their tokenisation.
    eval "set -- $ssh_options"
elif [[ $1 =~ .*@.* ]]; then
    # Use the connection information to build juju remotely in ~/juju-build.
    THERE="~"
    IS_EPHEMERAL="false"
    REMOTE_USER=$(echo $1 | cut -d @ -f1)
    INSTANCE_NAME=$(echo $1 | cut -d @ -f2)
    # Remap ssh option strings to $@ to preserve their tokenisation.
    eval "set -- $ssh_options $2"
else
    # Create an instance to build juju remotely in ~/juju-build.
    THERE="~"
    IS_EPHEMERAL="true"
    REMOTE_USER="ubuntu"
    export INSTANCE_TYPE=$1
    export AMI_IMAGE=$2
    # Remap ssh option strings to $@ to preserve their tokenisation.
    eval "set -- $ssh_options"
fi


if [[ $IS_EPHEMERAL == "true" ]]; then
    create_instance "$@"
fi
install_build_deps "$@"
upload_source_package_files "$@"
build_binary_packages "$@"
retrieve_binary_packages "$@"
cleanup "$@"
exit $EXIT_STATUS
