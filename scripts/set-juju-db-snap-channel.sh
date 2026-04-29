#!/bin/bash

set -e -u

export PS4='+ ${BASH_SOURCE:-}:${LINENO:-}:${FUNCNAME[*]}: '

declare controller=""
declare channel="4.4.30/stable"
declare -i debug=0
declare -a machines
declare -A machine_ips=()
declare -A machine_tags=()
declare -i primary=0
declare primary_ip=""

get_controllers() {
	local endpoints=()
	local controllers_file=~/.local/share/juju/controllers.yaml
	if [[ ! -f ${controllers_file} ]]; then
		echo "Cannot find Juju controllers file"
		echo "${controllers_file}"
		exit 1
	fi
	if [[ $(cat ${controllers_file} | yq '.controllers | has("'${controller}'")') == false ]]; then
		echo "The controller '${controller}' does not exist"
		exit 1
	fi
	readarray -t endpoints < <(cat ${controllers_file} | yq '.controllers."'${controller}'"."api-endpoints"[]')
	local pair
	local ip
	local id
	local tag
	for pair in ${endpoints[@]}; do
		ip=${pair%:*}
		ip=${ip/[/}
		ip=${ip/]/}
		id=$(mongo_cmd ${ip} 'var m = rs.status().members.find(m => m.name.includes("'${ip}'")); if (m) { print(m._id); }' | grep --only-matching --extended-regexp '^[0-9]+$' || echo "not found")
		if [[ ${id} =~ ^[0-9]+$ ]]; then
			machine_ips[${id}]=${ip}
			machines+=(${id})
			tag=($(run_remote_command ${ip} sudo awk "'"'/tag/ {print $2}'"'" /var/lib/juju/agents/machine-*/agent.conf))
			tag=${tag#machine-}
			machine_tags[${id}]=${tag}
		fi
	done
	echo "controllers:     ${machines[*]}"
	echo "IP addresses:    ${machine_ips[*]}"
	echo "Controller tags: ${machine_tags[*]}"
}

get_primary_ip() {
	local -a result=()
	local output
	echo -n "Identifying primary MongoDB member"
	for i in $(seq 10); do
		output=$(mongo_cmd ${machine_ips[${machines[0]}]} 'rs.status().members.find(m => m.state === 1).name')
		read -a result -d: < <(echo ${output} | grep -e ':37017$') || echo -n .
		if ((${#result[@]} > 0)); then
			break
		fi
		sleep 1
	done
	echo
	if ((${#result[@]} == 0)); then
		echo "Failed to identify primary by looking for '/:37017\$/'"
		echo "Last output: '${output}'"
		exit 1
	fi
	primary_ip="${result}"
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
	local machine
	local agent_version
	local agent_major_version
	for machine in ${machines[@]}; do
		agent_version=$(run_remote_command ${machine_ips[${machine}]} sudo cat /var/lib/juju/agents/machine-*/agent.conf | yq .upgradedToVersion)
		local agent_major_version=${agent_version%%.*}
		if ((agent_major_version == 2)); then
			if (($(compare_version ${agent_version} 2.9.57) < 0)); then
				echo "Controller ${machine_tags[${machine}]} is at version ${agent_version}; please upgrade the controller to >= 2.9.57 and then run this script again"
				exit 1
			fi
		else
			if (($(compare_version ${agent_version} 3.6.21) < 0)); then
				echo "Controller ${machine_tags[${machine}]} is at version ${agent_version}; please upgrade the controller to >= 3.6.21 and then run this script again"
				exit 1
			fi
		fi
		echo "Controller ${machine_tags[${machine}]} is at ${agent_version}"
	done
}

check_mongo_version() {
	local machine
	for machine in ${machines[@]}; do
		local mongo_version=$(mongo_cmd ${machine_ips[${machine}]} 'db.version()')
		local snap_channel=$(get_juju_db_snap_channel ${machine_ips[${machine}]})
		if (($(compare_version ${mongo_version} 4.4.30) < 0)); then
			echo "MongoDB ${machine_tags[${machine}]} is at version ${mongo_version} and vulnerable"
			return
		else
			echo "MongoDB ${machine_tags[${machine}]} is at version ${mongo_version} and not vulnerable"
		fi
		if [[ ${snap_channel} == error ]]; then
			continue
		fi
		if [[ ${snap_channel} != ${channel} ]]; then
			echo "juju-db using channel ${snap_channel} and needs to be upgraded to ${channel}"
			return
		fi
	done
	echo "No upgrades necessary"
	exit 0
}

run_remote_command() {
	local ip=$1
	shift
	local cmd="$*"
	local keyfile=~/.local/share/juju/ssh/juju_id_ed25519
	if [[ ! -f ${keyfile} ]]; then
		keyfile=~/.local/share/juju/ssh/juju_id_rsa
		if [[ ! -f ${keyfile} ]]; then
			echo "Cannot find Juju ssh key" >&2
			echo "Looked in ~/.local/share/juju/ssh/{juju_id_ed25519,juju_id_rsa}" >&2
			kill -TERM $$
			exit 1
		fi
	fi
	local err_target=/dev/stderr
	if ((debug == 0)); then
		err_target=/dev/null
	fi
	ssh -o StrictHostkeyChecking=no -i ${keyfile} ubuntu@${ip} -- "${cmd}" 2>${err_target}
}

shut_down_jujud() {
	local machine
	for machine in ${!machine_ips[@]}; do
		run_remote_command ${machine_ips[${machine}]} sudo systemctl stop jujud-machine-*.service
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
		run_remote_command ${machine_ips[${machine}]} sudo systemctl start jujud-machine-${machine_tags[${machine}]}.service
	done
}

mongo_cmd() {
	local ip=$1
	local cmd="$2"

	local quoted_cmd
	quoted_cmd=$(printf '%q' "${cmd}")
	read -r -d '' cmds <<EOF || true
if (( debug != 0 )); then
    set -x
fi
conf=/var/lib/juju/agents/machine-*/agent.conf
user=\$(sudo awk '/^tag:/ {print \$2}' \${conf})
password=\$(sudo awk '/^statepassword:/ {print \$2}' \${conf})
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
sudo \${client} --quiet localhost:37017/juju \\
    --authenticationDatabase admin \\
    --tls \\
    \${certs} \\
    --username "\${user}" --password "\${password}" \\
    --eval ${quoted_cmd}
EOF
	cmds="channel=${channel}
cmd=${quoted_cmd}
debug=${debug}
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
	local result=$(mongo_cmd ${ip} 'db.controllers.find({_id: "controllerSettings"}, {"settings.juju-db-snap-channel": 1})')
	if [[ ${result} =~ "not master" ]]; then
		echo 'error'
	else
		jq --raw-output '.settings."juju-db-snap-channel"' <<<${result}
	fi
}

upgrade_juju_db() {
	local machine=$1
	local channel="$2"

	echo "Upgrading juju-db snap on machine ${machine} to ${channel}"

	echo -n "Stopping juju-db: "
	run_remote_command ${machine_ips[${machine}]} sudo snap stop juju-db
	echo -n "Refreshing juju-db snap: "
	run_remote_command ${machine_ips[${machine}]} sudo snap refresh --channel=${channel} juju-db
	echo -n "Starting juju-db: "
	run_remote_command ${machine_ips[${machine}]} sudo snap start juju-db
}

print_help() {
	cat <<-EOF
		Usage:

		-h | --help         This help
		--debug             Add debugging output
		--controller NAME   The controller name
		--channel CHANNEL   The juju-db snap channel (default = ${channel})
	EOF
}

while (($# > 0)); do
	case $1 in
	-h | --help)
		print_help
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
	echo "Missing controller name; please run with '--controller name'"
	print_help
	exit 1
fi

if ((debug != 0)); then
	set -x
fi

echo "Upgrading MongoDB for controller '${controller}'"

if ! command -v yq >/dev/null; then
	echo "Please install the yq command before proceeding"
	echo "sudo snap install yq"
	exit 1
fi

get_controllers
get_primary_ip
check_juju_version
check_mongo_version
shut_down_jujud

for machine in ${!machine_ips[@]}; do
	echo "Upgrading machine ${machine_tags[${machine}]}"
	upgrade_juju_db ${machine} ${channel}
done

# Since we cycled the juju-db snaps, we need to find the current primary.
get_primary_ip

update_juju_db_snap_channel ${primary_ip} ${channel}
echo "The new juju-db-snap-channel is $(get_juju_db_snap_channel ${primary_ip})"

# Start agents
start_jujud

echo "Sucessfully upgraded juju-db"
