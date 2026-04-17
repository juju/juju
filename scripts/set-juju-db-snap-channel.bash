#!/bin/bash

set -e -u

export PS4='+ ${BASH_SOURCE:-}:${LINENO:-}:${FUNCNAME[0]:-}: '

declare controller=""
declare channel="4.4.30/stable"
declare juju=$(command -v juju)
declare -i debug=0
declare -A machine_ips=()
declare -i primary=0

get_controller_machines() {
    local output=$(${juju} show-controller | yq .${controller}.'"controller-machines"')
    local machines
    readarray -t machines < <(yq '. | keys | .[]' <<< "${output}")
    echo "found ${#machines[@]} controller machine(s)"
    primary=$(yq '[to_entries | .[] | select(.value."ha-primary" == true) | .key] | .[0] // (keys | .[0])' <<< "${output}")
    local -a ipaddresses=()
    for machine in ${machines[@]}; do
        readarray -t ipaddresses < <(juju show-machine --model controller | yq .machines['"'${machine}'"'].'"ip-addresses"[]')
        machine_ips[${machine}]=${ipaddresses[0]}
    done
}

get_primary_ip() {
    local -a result=()
    for i in $(seq 10); do
        read -a result -d: < <(mongo_cmd ${machine_ips[0]} 'rs.status().members.find(m => m.state === 1).name' | grep -e ':37017$')
        if [[ -n ${result} ]]; then
            break
        fi
        sleep 1
    done
    if [[ -z ${result} ]]; then
        echo "Failed to identify primary"
        exit 1
    fi
    echo "${result}"
}

compare_version() {
    local v1=$1
    local v2=$2
    if [[ ${v1} == ${v2} ]]; then
        echo 0
        return
    fi
    if [[ $(printf "%s\n%s" ${v1} ${v2} | sort --version-sort | tail --lines 1) == ${v2} ]]; then
        echo -1
    else
        echo +1
    fi
}

check_juju_version() {
    local output=$(${juju} show-controller | yq .${controller}.details)
    local model_version=$(yq '."controller-model-version"' <<< "${output}")
    local model_major_version=$(awk -F. '{print $1}' <<< "${model_version}")
    local mongo_version=$(yq '."mongo-version"' <<< "${output}")
    if (( $(compare_version ${mongo_version} 4.4.30) < 0 )); then
        echo "MongoDB is at version ${mongo_version} and vulnerable"
    else
        echo "MongoDB is at version ${mongo_version} and not vulnerable"
        exit 0
    fi
    if (( model_major_version == 2 )); then
        if (( $(compare_version ${model_version} 2.9.56) < 0 )); then
            echo "Controller is at ${model_version}; please upgrade the controller to >= 2.9.56 and then run this script again"
            exit 1
        fi
    else
        if (( $(compare_version ${model_version} 3.6.19) < 0 )); then
            echo "Controller is at ${model_version}; please upgrade the controller to >= 3.6.19 and then run this script again"
            exit 1
        fi
    fi
}

run_remote_command() {
    local ip=$1
    shift
    local cmd="$*"
    local keyfile=~/.local/share/juju/ssh/juju_id_ed25519
    if [[ ! -f ${keyfile} ]]; then
        keyfile=~/.local/share/juju/ssh/juju_id_rsa
    fi
    ssh -i ${keyfile} ubuntu@${ip} -- "${cmd}"
}

shut_down_jujud() {
    local machine
    for machine in ${!machine_ips[@]}; do
        run_remote_command ${machine_ips[${machine}]} sudo systemctl stop jujud-machine-${machine}.service
    done
}

shut_down_juju_db() {
    local machine=$1
    run_remote_command ${machine_ips[${machine}]} sudo snap stop juju-db
}

shut_down_secondaries() {
    local machine
    for machine in ${!machine_ips[@]}; do
        if [[ ${machine} == ${primary} ]]; then
            continue
        fi
        run_remote_command ${machine_ips[${machine}]} sudo snap stop juju-db
    done
}

start_jujud() {
    local machine
    for machine in ${!machine_ips[@]}; do
        run_remote_command ${machine_ips[${machine}]} sudo systemctl start jujud-machine-${machine}.service
    done
}

mongo_cmd() {
    local ip=$1
    local cmd="$2"

    local quoted_cmd
    quoted_cmd=$(printf '%q' "${cmd}")
    read -r -d '' cmds <<EOF || true
set -x
conf=/var/lib/juju/agents/machine-*/agent.conf
user=\$(sudo awk '/tag/ {print \$2}' \${conf})
password=\$(sudo awk '/statepassword/ {print \$2}' \${conf})
if [[ -f /snap/bin/juju-db.mongo ]]; then
    client=/snap/bin/juju-db.mongo
elif [[ -f /usr/lib/juju/mongo*/bin/mongo ]]; then
    client=/usr/lib/juju/mongo*/bin/mongo
else
    client=/usr/bin/mongo
fi
certs=--tlsAllowInvalidCertificates
if sudo test -f /var/snap/juju-db/common/ca.crt; then
    certs="--tlsCertificateKeyFile=/var/snap/juju-db/common/server.pem --tlsCAFile=/var/snap/juju-db/common/ca.crt"
fi
sudo \${client} localhost:37017/juju \\
    --authenticationDatabase admin \\
    --tls \\
    \${certs} \\
    --username "\${user}" --password "\${password}" \\
    --eval ${quoted_cmd}
EOF
    cmds="channel=${channel}
cmd=${quoted_cmd}
${cmds}"
    run_remote_command ${ip} "${cmds}"
}

update_juju_db_snap_channel() {
    local ip=$1
    local channel=$2

    echo "Upgrading MongoDB on MongoDB primary to ${channel}"

    mongo_cmd ${ip} 'db.controllers.update({_id: "controllerSettings"}, {$set: {"settings.juju-db-snap-channel": "'"${channel}"'"}})'
}

get_juju_db_snap_channel() {
    local ip=$1
    mongo_cmd ${ip} 'db.controllers.find({_id: "controllerSettings"}, {"settings.juju-db-snap-channel": 1})'
}

upgrade_juju_db() {
    local machine=$1
    local channel="$2"

    echo "Upgrading juju-db snap on machine ${machine} to ${channel}"

    run_remote_command ${machine_ips[${machine}]} sudo snap stop juju-db
    run_remote_command ${machine_ips[${machine}]} sudo snap refresh --channel=${channel} juju-db
}

while (( $# > 0 )); do
    case $1 in
        -h|--help)
            cat <<-EOF
Usage:

-h | --help         This help
--debug             Add debugging output
--controller NAME   The controller name
--channel CHANNEL   The juju-db snap channel (default = ${channel})
EOF
            exit 0
            ;;
        --debug)
            debug=1
            ;;
        --controller)
            shift
            controller=$1
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

if [[ -z ${controller} ]]; then
    echo "Missing controller name"
    exit 1
fi

if (( debug != 0 )); then
    set -x
fi

echo "Upgrading controller '${controller}'"

if ! juju show-controller ${controller} > /dev/null; then
    echo "Failed to access controller ${controller}"
    exit 1
fi

${juju} switch ${controller}
get_controller_machines
check_juju_version

shut_down_jujud

for machine in ${!machine_ips[@]}; do
    echo "Upgrading machine ${machine}"
    upgrade_juju_db ${machine} ${channel}
done

# Since we cycled the juju-db snaps, we need to find the current primary.
primary_ip=$(get_primary_ip)

update_juju_db_snap_channel ${primary_ip} ${channel}
get_juju_db_snap_channel ${primary_ip}

# Start agents
start_jujud
