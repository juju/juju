#!/usr/bin/env bash

# Until we get the featuretests to clean up properly, this script
# should be used to clean up.  Once the tests are fixed this script
# may be removed.
# TODO(ericsnow) Remove this script.

DIV="---------------------------------"

UPSTART="/sbin/initctl"
SYSTEMD="/sbin/systemctl"

ENV="local"

ID=${USER}-${ENV}
if [ -n "$1" ]; then
    ID=$1-${ENV}
fi

SVC_NAME="juju-%s-${ID}"

INIT=""
if [ -f $UPSTART ]; then
    INIT=$UPSTART
elif [ -f $SYSTEMD ]; then
    INIT=$SYSTEMD
fi


function run  {
    local CMD="sudo $1"
    echo $DIV
    echo running '"'${CMD}'"'
    exec ${CMD}
}


function remove_service {
    local KIND=$1
    local INIT=$2
    local NAME=$(printf $SVC_NAME $KIND)

    #run "$INIT stop $NAME"
    run "service $NAME stop"

    if [ "$INIT" = $UPSTART ]; then
        run "rm -rf /etc/init/${NAME}.conf"
    elif [ "$INIT" = $SYSTEMD ]; then
        run "$SYSTEMD disable $NAME"
        run "find /etc/systemd -name \*{NAME}\* -exec rm -rf {} \;"
    else
        echo $DIV
        echo "- unrecognized init system "'"'$INIT'"'
        return
    fi

    if [ "$INIT" = $UPSTART ]; then
        run "$UPSTART reload-configuration"
    elif [ "$INIT" = $SYSTEMD ]; then
        run "$SYSTEMD daemon-reload"
    fi
}


# Clean up the services
remove_service "db" $INIT
remove_service "agent" $INIT

# Clean up the logs.
run "rm -rf /var/log/juju-${ID}"

# Clean up the JUJU_HOME dir.
run "rm -rf /tmp/juju-test-${ENV}-*"

# Clean up the containers, if any.
run "lxc-stop ${ID}-machine-1"
run "lxc-destroy ${ID}-machine-1"
