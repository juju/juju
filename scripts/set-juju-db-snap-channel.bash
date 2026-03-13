#!/bin/bash

set -e -u -x

declare -i machine=0
declare channel="4.4/stable"

declare juju=$(command -v juju)

mongo_command() {
    local machine=$1
    local channel="$2"

    read -r -d '' cmds <<'EOF' || true
conf=/var/lib/juju/agents/machine-*/agent.conf
user=$(sudo awk '/tag/ {print $2}' ${conf})
password=$(sudo awk '/statepassword/ {print $2}' ${conf})
if [ -f /snap/bin/juju-db.mongo ]; then
    client=/snap/bin/juju-db.mongo
elif [ -f /usr/lib/juju/mongo*/bin/mongo ]; then
    client=/usr/lib/juju/mongo*/bin/mongo
else
    client=/usr/bin/mongo
fi
sudo systemctl stop jujud-machine-${machine}.service
${client} 127.0.0.1:37017/juju --authenticationDatabase admin \
    --ssl --sslAllowInvalidCertificates \
    --username "${user}" --password "${password}" \
    --eval 'db.controllers.find({_id: "controllerSettings"}, {"settings.juju-db-snap-channel": 1});
        db.controllers.update({_id: "controllerSettings"}, {$set: {"settings.juju-db-snap-channel": "'"${channel}"'"}});
        db.controllers.find({_id: "controllerSettings"}, {"settings.juju-db-snap-channel": 1})'
sudo systemctl start jujud-machine-${machine}.service
sudo snap refresh --channel=${channel} juju-db
EOF
    cmds="machine=${machine}
channel=${channel}
${cmds}"
    ${juju} ssh --model controller "${machine}" "${cmds}"
}

while (( $# > 0 )); do
    case $1 in
        -h|--help)
            cat <<-EOF
Usage:

-h | --help         This help
--machine ID        The machine ID in the controller model (default = ${machine})
--channel CHANNEL   The juju-db snap channel (default = ${channel})
EOF
            exit 0
            ;;
        --machine)
            shift
            machine=$(($1))
            ;;
        --channel)
            shift
            channel="$1"
            ;;
        *)
            echo "unknown argument $1"
            exit 1
            ;;
    esac
    shift
done

mongo_command ${machine} "${channel}"

# db.controllers.update({_id: "controllerSettings"}, {$set: {"settings.juju-db-snap-channel": "4.4/candidate"}})
