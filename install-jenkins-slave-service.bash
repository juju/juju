#!/bin/bash
# Install and config jenkins slave charm without Juju
# The juju-qa jenkins-slave sets up jenkins as CI needs,
# but some machines cannot be provisioned by Juju because
# the network does not have the egress.
#
# This script downloads the charm and forces it to install.
# The setup-slave.bash script can be run afterwards.
#
# install-jenkins-slave.bash user@host slave-name http://user:pass@master:8080
set -eu

USER_AT_ADDRESS=$1
SLAVE_NAME=$2
MASTER_URL=$3


function make-charm-cmds() {
    if [[ ! -d $CHARM_CMDS ]]; then
        mkdir -p $CHARM_CMDS
    fi
    cat <<EOT > $CHARM_CMDS/status-set
#!/bin/bash 
echo "\$@"
EOT

    cat <<EOT > $CHARM_CMDS/relation-ids
#!/bin/bash 
echo ""
EOT

    cat <<EOT > $CHARM_CMDS/config-get
#!/bin/bash 
option="\$1"
if [[ \$option == "slave-name" ]]; then
    echo "$SLAVE_NAME"
elif [[ \$option == "master-url"  ]]; then
    echo "$MASTER_URL"
else
    echo ""
fi
EOT
chmod +x $CHARM_CMDS/*
ls -l $CHARM_CMDS
}


function install-jenkins() {
    set -eu
    export CHARM_CMDS=$HOME/charm-cmds
    export JUJU_CHARM_DIR=$HOME/charm-jenkins-slave
    export SLAVE_NAME=$SLAVE_NAME
    export MASTER_URL=$MASTER_URL

    # sudo apt-get install unzip
    # wget -O $HOME/jenkins-slave.zip \
    #     https://api.jujucharms.com/charmstore/v5/~juju-qa/jenkins-slave/archive
    # unzip jenkins-slave.zip -d $JUJU_CHARM_DIR
    # make-charm-cmds 

    # # Force the bash functions into root user's env
    # sudo -E $JUJU_CHARM_DIR/hooks/install
    # Force the bash functions into root user's env
    sudo -E PATH=$CHARM_CMDS:$PATH $JUJU_CHARM_DIR/hooks/config-changed
}


ssh $USER_AT_ADDRESS "$(typeset -f); \
    SLAVE_NAME=$SLAVE_NAME MASTER_URL=$MASTER_URL install-jenkins"
