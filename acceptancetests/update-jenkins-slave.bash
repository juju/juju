#!/bin/bash
set -eux


update_branch() {
    # Branch or pull a branch.
    local_branch=$1
    local_dir="$(basename $local_branch | cut -d ':' -f2)"
    local_path="$HOME/$local_dir"
    if [[ -d $local_path ]]; then
        # Clear any changes that haven't been commited to the branch.
        (cd $local_path; bzr revert; bzr pull)
    else
        bzr branch $local_branch $local_path
    fi
}


update_git_repo() {
    # Clone or pull a git repo.
    git_repo=$1
    branch=$2
    local_dir=$3
    local_path="$HOME/$local_dir"
    if [[ -d $local_path ]]; then
        # Clear any local changes
        if [[ -d "$local_path/.bzr" ]]; then
            # check for .bzr, if so, do fresh clone
            echo "Found bzr repo, removing and cloning"
            git clone $git_repo "$local_path-git"
            rm -rf $local_path
            mv "$local_path-git" "$local_path"
        else
            cd $local_path
            git reset --hard origin/$branch
            # git reset breaks file permissions, which will break the pull
            chmod 600 $HOME/cloud-city/staging-juju-rsa
            chmod 600 $HOME/.ssh/id_rsa
            git pull $git_repo

        fi
    else
        git clone $git_repo $local_path
    fi
}


get_os() {
    # Get the to OS name: ubuntu, darwin, linux, unknown.
    local_uname=$(uname -a)
    if [[ "$local_uname" =~ ^.*Ubuntu.*$ ]]; then
        echo "ubuntu"
    elif [[ "$local_uname" =~ ^.*Darwin.*$ ]]; then
        echo "darwin"
    elif [[ "$local_uname" =~ ^.*Linux.*$ ]]; then
        # Probably CentOS.
        echo "linux"
    else
        echo "unknown"
    fi
}

echo "Updating branches"
OS=$(get_os)

# Make sure we have access to the machine
if [[ $OS == "ubuntu" ]]; then
    echo "Importing ssh ids for juju-qa-bot"
    ssh-import-id lp:juju-qa-bot
fi

if [[ $OS != "ubuntu" ]]; then
    hammertime="disabled"
# Does not support python3-venv
elif (lsb_release -c|grep -E 'trusty|precise'); then
    hammertime="disabled"
# Check network access to github
elif (netcat github.com 22 -w 1 > /dev/null < /dev/null); then
    hammertime="enabled"
else
    hammertime="disabled"
fi

update_branch lp:workspace-runner
update_git_repo https://github.com/juju/juju.git develop juju
update_git_repo git+ssh://juju-qa-bot@git.launchpad.net/~juju-qa/+git/cloud-city master cloud-city
if [[ $hammertime == "enabled" ]]; then
    update_git_repo https://github.com/juju/hammertime.git master hammertime
fi

echo "Updating permissions"
sudo chown -R jenkins $HOME/cloud-city
chmod -R go-rwx $HOME/cloud-city
chmod 700 $HOME/cloud-city/gnupg
chmod 600 $HOME/cloud-city/staging-juju-rsa

echo "Updating dependencies from branches"
if [[ $OS == "ubuntu" ]]; then
    make -C $HOME/juju-ci-tools install-deps
    make -C $HOME/workspace-runner install
elif [[ $OS == "darwin" ]]; then
    $HOME/juju-ci-tools/pipdeps.py install
fi
if [[ $hammertime == "enabled" ]]; then
    make -C $HOME/hammertime develop
fi

echo "For legacy purposes, populate to legacy directories"
echo "Copying acceptancetests -> juju-ci-tools"
rsync -avz --delete $HOME/juju/acceptancetests/ $HOME/juju-ci-tools/

echo "Copying releasetests -> juju-release-tools"
rsync -avz --delete $HOME/juju/acceptancetests/ $HOME/juju-release-tools/

echo "Copying acceptancetests/repository -> repository"
rsync -avz --delete $HOME/juju/acceptancetests/repository/ $HOME/repository/


echo "$HOSTNAME update complete"
