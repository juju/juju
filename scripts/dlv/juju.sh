#!/usr/bin/env bash
set -e

# This script create a ssh tunnel between the local computer to a juju debug socket open in a controller or machine.
# By default, it allows to enable debug on controller machine 0 through the local port 2345.
#
# Usage:
#   [CMD] [Options]
#
#
# Options:
#   -c controller_name : allows to specify the controller. Defaulted to the current one.
#   -m model_name : allows to target a specific model (to connect to specific machine). Defaulted to 'controller'.
#   -u machine_id : allows to target a specific machine. Defaulted to 0.
#   -l port : allows to specify the port. Defaulted to 2345
#   -s socket_path : allows to specify a specific debug socket in the machine. If there is only one, it will be
#                    defaulted. If there is several available (for instance because there is several running juju
#                    binaries on the machine), a list will be printed and this flag will allows to select the right one.
#   -i identity_file_path : allows to select the private key to connect to the machine. By default it will use the one
#                           here: ${HOME}/.local/share/juju/ssh/juju_id_ed25519", but it can be necessary to use another
#                           one in some use cases.
#   -d : if defined, run in detached mode. Will print the pid of the created ssh tunnel to be able to kill it later.

# Parse options
while getopts "c:m:u:l:s:i:d" opt; do
    case $opt in
    d)
        detach=1
        ;;
    c)
        target_controller=$OPTARG
        ;;
    m)
        target_model=$OPTARG
        ;;
    u)
        target_machine=$OPTARG
        ;;
    l)
        target_port=$OPTARG
        ;;
    s)
        target_socket=$OPTARG
        ;;
    i)
        identity_key=$OPTARG
        ;;
    *)
        echo "Unknown option"
        exit 1
        ;;
    esac
done

# setup default values
target_controller=${target_controller:-$(juju whoami --format=json | jq -r '.controller')}
target_port=${target_port:-2345}
target_model=${target_controller}:${target_model:-controller}
target_machine=${target_machine:-0}

if [ -z $identity_key ]; then
    if [ -f "${HOME}/.local/share/juju/ssh/juju_id_ed25519" ]; then
        identity_key="${HOME}/.local/share/juju/ssh/juju_id_ed25519"
    elif [ -f "${HOME}/.local/share/juju/ssh/juju_id_rsa" ]; then
        identity_key="${HOME}/.local/share/juju/ssh/juju_id_rsa"
    else
        echo "no default key found in" "${HOME}/.local/share/juju/ssh"
        exit 1
    fi
fi

# find out the remote host to ssh to
remote_host=$(juju status -m ${target_model} --format=json | jq -r ".machines[\"${target_machine}\"][\"ip-addresses\"][0]")
remote_host=${remote_host%%/*}

if [ -z $target_socket ]; then
    echo "No target specified, trying to figure it out"

    # SSH command to find .socketd files
    cmd="find /var/lib/juju/ -type s -name '*.socketd' 2> /dev/null || true"

    # Run the command over SSH and capture the output
    output=$(ssh -o StrictHostKeyChecking=no -i ${identity_key} "ubuntu@${remote_host}" "$cmd")

    # Capture the number of results
    result_count=$(echo "$output" | wc -l)

    if [ "$result_count" -eq 0 ]; then
        echo "No socket found"
        exit 1
    elif [ "$result_count" -eq 1 ]; then
        # If there is exactly one result, use it
        target_socket=$output
    else
        # Otherwise, print the list of found sockets
        echo "Multiple files found:"
        echo "$output"
        exit 1
    fi
fi

echo "Starting dlv proxy to ${target_model} (${remote_host}:${target_port}) - machine ${target_machine}"

tunnel_cmd="ssh -N -o StrictHostKeyChecking=no -i ${identity_key} -L127.0.0.1:${target_port}:${target_socket} ubuntu@${remote_host}"

if [ -z $detach ]; then
    /usr/bin/bash -c "${tunnel_cmd}"
else
    /usr/bin/bash -c "${tunnel_cmd}" &
fi

pid=$(pgrep -fa ubuntu@${remote_host} | cut -d ' ' -f 1)
echo "dlv proxy started (to stop proxy: kill ${pid})"
