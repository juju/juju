#!/bin/bash

# IMPORTANT: The ssh options are always remapped to $@ which preserves
# their tokenisation.


usage() {
    echo "usage: $0 [<instance-type> <ami> | <[user@]host> '<ssh-optons>'] dsc"
    echo "  Build binary packages on an ec2 instance or a remote host."
    echo ""
    echo "  instance-type: the ec2 instance type, like m1.large."
    echo "  ami: The ec2 image, it defines the series and arch to build."
    echo "  user@host: host to build on. 'user@' is optional."
    echo "      Use localhost to build locally."
    echo "  ssh-option: a string of all the options to establish a connection."
    echo "      Like '-o \"ProxyCommand ssh user@10.10.10.10 nc -q0 %h %p\"'."
    echo "  dsc: The path the the dsc source file."
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
SCRIPTS=$(dirname $0)
CHANGES=$(readlink -f $3)

if [[ $1 = "localhost" ]]; then
    # Use the local user and working directory to build juju.
    is_ephemeral="false"
    THERE=$(pwd)
    remote_user=$USER
    instance_name=$1
    eval "set -- $ssh_options"
elif [[ $1 =~ .*@.* ]]; then
    # Use the connection information to build juju remotely in ~/juju-build.
    THERE="~"
    is_ephemeral="false"
    remote_user=$(echo $1 | cut -d @ -f1)
    instance_name=$(echo $1 | cut -d @ -f2)
    eval "set -- $ssh_options $2"
else
    # Create an instance to build juju remotely in ~/juju-build.
    THERE="~"
    is_ephemeral="true"
    remote_user="ubuntu"
    export INSTANCE_TYPE=$1
    export AMI_IMAGE=$2
    eval "set -- $ssh_options"
fi


if [[ $is_ephemeral == "true" ]]; then
    echo "Creating an instance to build juju on."
    instance_id=$($SCRIPTS/ec2-run-instance-get-id)
    $SCRIPTS/ec2-tag-job-instances $instance_id
    set +x
    echo Starting instance $instance_id
    instance_name=$($SCRIPTS/ec2-get-name $instance_id)
    echo Instance has ip $instance_name
    sleep 30
    $SCRIPTS/wait-for-port $instance_name 22
    # Pause, do not start until cloud-init has updated the apt sources.
    ssh "$@" $remote_user@$instance_name  <<EOT
    set -eux
    if [[ $is_ephemeral == "true" ]]; then
        for attempt in \$(seq 10); do
            if grep -r '^deb .* universe$' /etc/apt/sources.list > /dev/null
            then
                break
            elif [ "\$attempt" == "10" ]; then
                echo "Universe is not available to install packages from."
                exit 1
            fi
            sleep 10m
        done
    fi
EOT
fi


echo "Installing build deps."
remote_compliler=$(ssh "$@" $remote_user@$instance_name  <<EOT
    if [[ \$(uname -p) =~  .*86|armel|armhf.* ]]; then
        echo "golang"
    else
        echo "gccgo-4.9 gccgo-go"
    fi
EOT
)
juju_compliler=$(echo "$remote_compliler" | tail -1)
DEPS="build-essential fakeroot dpkg-dev debhelper $juju_compliler"

dep_script=$(cat <<EOT
    sudo apt-add-repository -y ppa:juju/golang;
    sudo apt-get update;
    sudo apt-get install -y $DEPS;
EOT
)

if [[ $instance_name = "localhost" ]]; then
    eval "$dep_script"
else
    ssh "$@" $remote_user@$instance_name  <<EOT
    set -eux
    eval "$dep_script"
EOT
fi


echo "Uploading source package files."
source_dir=$(dirname $CHANGES)
source_file_data=$(
    sed -n '/^Checksums-Sha256/,/^Files/!d; /^ /!d; s,^ ,,; s, ,:,gp' $CHANGES)
source_files="$CHANGES"
echo "Found these files in $source_dir to upload:"
for data in $source_file_data
do
    file_sha256=$(echo "$data" | cut -d : -f 1)
    file_size=$(echo "$data" | cut -d : -f 2)
    file_name=$(echo "$data" | cut -d : -f 3)
    echo "$file_name"
    source_files="$source_files $source_dir/$file_name"
done
ssh "$@" $remote_user@$instance_name <<EOT
    test -d $THERE/juju-build/ || mkdir $THERE/juju-build
EOT
scp "$@" $source_files $remote_user@$instance_name:$THERE/juju-build/


echo "Building binary packages"
juju_version=$(basename $CHANGES _source.changes)
version=$(echo $juju_version | cut -d _ -f2 | cut -d - -f 1)
ssh "$@" $remote_user@$instance_name <<EOT
    set -eux
    cd $THERE/juju-build/
    dpkg-source -x $juju_version.dsc
    cd juju-core-$version
    dpkg-buildpackage -us -uc
EOT


echo "Retrieving built packages."
scp "$@" $remote_user@$instance_name:$THERE/juju-build/*.deb ./
EXIT_STATUS=$?


echo "Cleaning up remote"
if [[ $is_ephemeral == "true" ]]; then
    $SCRIPTS/ec2-terminate-job-instances
else
    ssh "$@" $remote_user@$instance_name  "rm -r $THERE/juju-build"
fi

exit $EXIT_STATUS
