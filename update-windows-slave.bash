#!/bin/bash
# Update a windows slave
set -eux

HERE=$(pwd)
SCRIPTS=$(readlink -f $(dirname $0))
REPOSITORY_PARENT=$(dirname $SCRIPTS)

USER_AT_HOST=$1


update_windows() {
    user_at_host=$1
    scp repository.zip $user_at_host:/cygdrive/c/Users/Administrator/
    ssh $user_at_host << EOT
bzr pull -d ./juju-release-tools
bzr pull -d ./juju-ci-tools
/cygdrive/c/progra~2/7-Zip/7z.exe x -y repository.zip
python ./juju-ci-tools/pipdeps.py install
EOT
}


# win-slaves have a different user and directory layout than POSIX hosts.
# Also, win+bzr does not support symlinks, so we zip the local charm repo.
(cd $REPOSITORY_PARENT; zip -q -r $HERE/repository.zip repository -x *.bzr*)
# The ssh connection to the host is unreliable so it is tried twice.
update_windows $USER_AT_HOST || update_windows $USER_AT_HOST
