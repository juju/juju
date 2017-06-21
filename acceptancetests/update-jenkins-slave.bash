#!/bin/bash
set -eux

#make sure we have proper known hosts
ssh-keyscan -H bazaar.launchpad.net git.launchpad.net upload.launchpad.net ppa.launchpad.net github.com  104.130.14.184 feature-slave-a.vapour.ws feature-slave-b.vapour.ws feature-slave-c.vapour.ws feature-slave-d.vapour.ws feature-slave-e.vapour.ws long-running-test.vapour.ws trusty-slave.vapour.ws xenial-slave.vapour.ws yakkety-slave.vapour.ws zesty-slave-a.vapour.ws release-slave.vapour.ws ci-master cwr-slave.vapour.ws juju-core-slave.vapour.ws juju-core-slave-b.vapour.ws charm-bundle-slave.vapour.ws cloud-health-slave.vapour.ws pencil-slave.vapour.ws certification-slave.vapour.ws build-slave canonistack-slave.vapour.ws feline-kiss finfolk munna silcoon arm64-slave.vapour.ws s390x-slave ppc64el-slave  >>~/.ssh/known_hosts
sort -u ~/.ssh/known_hosts > /tmp/known_hosts
cat /tmp/known_hosts > ~/.ssh/known_hosts

update_branch() {
    # Branch or pull a branch.
    local_branch=$1
    local_dir="$(basename $local_branch | cut -d ':' -f2)"
    local_path="$HOME/$local_dir"
    if [[ -d $local_path ]]; then
        # Clear any changes that have not been commited to the branch.
        (cd $local_path; bzr revert; bzr pull)
        cd ..
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
            cd ..
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

echo "Setting ssh key permissions"
chmod 600 $HOME/cloud-city/staging-juju-rsa


echo "Updating branches"
OS=$(get_os)

# Make sure we have access to the machine
if [[ $OS == "ubuntu" ]]; then
    echo "Importing ssh ids for juju-qa-bot"
    ssh-import-id lp:juju-qa-bot
fi


# Check network access
# We assume network access for unknown (windows)
if [[ $OS == "ubuntu" ]]; then
    restrictednetwork=false
    restrictedcanonicalnetwork=false
else
    if (nc github.com 22 -w 1 > /dev/null < /dev/null); then
        restrictednetwork=false
    else
        restrictednetwork=true
    fi

    if (nc git.launchpad.net 22 -w 1 > /dev/null < /dev/null); then
        restrictedcanonicalnetwork=false
    else
        restrictedcanonicalnetwork=true
    fi
fi

# special slaves, treat as restrictedcanonicalnetwork too
# munnas key does not let it grab cloud-city
if [[ $(hostname) == "munna" ]]; then
    restrictedcanonicalnetwork=true
fi

# slaves that require updates be pushed from the jenkins job
if [[ $(hostname) == "sinzui-jenkins2" ]] || [[ $(hostname) == "prodstack-slave-a" ]] || [[ $(hostname) == "prodstack-slave-b" ]]; then
    echo "Slave cannot see any other slaves. Jenkins job will push update"
    syncinjenkins=true
else
    syncinjenkins=false
fi


# Check hammertime support
hammertimerepo="disabled"
if [[ $OS = "ubuntu" ]] && [ "$restrictedcanonicalnetwork" = false ] && [ "$restrictednetwork" = false ]; then
    hammertimerepo="enabled"
    # Does not support python3-venv
    if (lsb_release -c|grep -E 'trusty|precise'); then
        hammertimerepo="disabled"
    fi
fi

if [ "$restrictedcanonicalnetwork" = false ]; then
    update_git_repo git+ssh://juju-qa-bot@git.launchpad.net/~juju-qa/+git/cloud-city master cloud-city
else
    if [ "$syncinjenkins" = true ]; then
        echo "Skipping. Jenkins job pushes instead"
    else
        echo "Cannot reach git.launchpad.net, copying from slave who can"
        # we use 10.125.0.13, which is silcoon
        rsync -avz --delete jenkins@10.125.0.13:/var/lib/jenkins/cloud-city/ $HOME/cloud-city/
    fi
fi

echo "Re-setting cloud-city permissions"
sudo chown -R jenkins $HOME/cloud-city
chmod -R go-rwx $HOME/cloud-city
chmod 700 $HOME/cloud-city/gnupg
chmod 600 $HOME/cloud-city/staging-juju-rsa


if [ "$restrictednetwork" = false ]; then
    update_git_repo https://github.com/juju/juju.git develop juju

    echo "For legacy purposes, populate to legacy directories"
    echo "Copying acceptancetests -> juju-ci-tools"
    rsync -avz --delete --exclude='*.pyc' $HOME/juju/acceptancetests/ $HOME/juju-ci-tools/

    echo "Copying releasetests -> juju-release-tools"
    rsync -avz --delete --exclude='*.pyc' $HOME/juju/releasetests/ $HOME/juju-release-tools/

    echo "Copying acceptancetests/repository -> repository"
    rsync -avz --delete --exclude='*.pyc' $HOME/juju/acceptancetests/repository/ $HOME/repository/

    if [[ $hammertimerepo == "enabled" ]]; then
        update_git_repo https://github.com/juju/hammertime.git master hammertime
    fi
else
    if [ "$syncinjenkins" = true ]; then
        echo "Skipping. Jenkins job pushes instead"
    else
        echo "Cannot reach github, copying from slave who can"
        # we use 162.213.35.54, which is ci-gateway
        echo "For legacy purposes, populate to legacy directories"
        echo "Copying acceptancetests -> juju-ci-tools"
        rsync -avz --delete --exclude='*.pyc' ubuntu@162.213.35.54:/home/ubuntu/juju/acceptancetests/ $HOME/juju-ci-tools/

        echo "Copying releasetests -> juju-release-tools"
        rsync -avz --delete --exclude='*.pyc' ubuntu@162.213.35.54:/home/ubuntu/juju/releasetests/ $HOME/juju-release-tools/

        echo "Copying acceptancetests/repository -> repository"
        rsync -avz --delete --exclude='*.pyc' ubuntu@162.213.35.54:/home/ubuntu/juju/acceptancetests/repository/ $HOME/repository/

        echo "Copying hammertime repository"
        rsync -avz --delete --exclude='*.pyc' ubuntu@162.213.35.54:/home/ubuntu/hammertime/ $HOME/hammertime/
    fi
fi


echo "$HOSTNAME update complete"
