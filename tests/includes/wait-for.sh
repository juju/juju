# SHORT_TIMEOUT is the time between polling attempts.
SHORT_TIMEOUT=5

# wait_for defines the ability to wait for a given condition to happen in a
# juju status output. The output is JSON, so everything that the API server
# knows about should be valid.
# The query argument is a jq query.
# The default timeout is 10 minutes. You can change this by providing the
# timeout argument (an integer number of seconds).
#
# ```
# wait_for <model name> <query> [<timeout>]
# ```
wait_for() {
	local name query timeout

	name=${1}
	query=${2}
	timeout=${3:-600} # default timeout: 600s = 10m

	attempt=0
	start_time="$(date -u +%s)"
	# shellcheck disable=SC2046,SC2143
	until [[ "$(juju status --format=json 2>/dev/null | jq -S "${query}" | grep "${name}")" ]]; do
		echo "[+] (attempt ${attempt}) polling status for" "${query} => ${name}"
		juju status --relations 2>&1 | sed 's/^/    | /g'
		sleep "${SHORT_TIMEOUT}"

		elapsed=$(date -u +%s)-$start_time
		if [[ ${elapsed} -ge ${timeout} ]]; then
			echo "[-] $(red 'timed out waiting for')" "$(red "${name}")"
			echo "    (controller) juju debug-log output"
			juju debug-log -m controller --replay --no-tail 2>&1 | sed 's/^/    | /g'
			echo "    (model) juju debug-log output"
			juju debug-log --replay --no-tail 2>&1 | sed 's/^/    | /g'
			exit 1
		fi

		attempt=$((attempt + 1))
	done

	if [[ ${attempt} -gt 0 ]]; then
		echo "[+] $(green 'Completed polling status for')" "$(green "${name}")"
		juju status --relations 2>&1 | sed 's/^/    | /g'
		# Although juju reports as an idle condition, some charms require a
		# breathe period to ensure things have actually settled.
		sleep "${SHORT_TIMEOUT}"
	fi
}

idle_condition() {
	local name app_index unit_index

	name=${1}
	app_index=${2:-0}
	unit_index=${3:-0}

	path=".[\"$name\"] | .units | .[\"$name/$unit_index\"]"

	echo ".applications | select(($path | .[\"juju-status\"] | .current == \"idle\") and ($path | .[\"workload-status\"] | .current != \"error\")) | keys[$app_index]"
}

active_idle_condition() {
	local name app_index unit_index

	name=${1}
	app_index=${2:-0}
	unit_index=${3:-0}

	path=".[\"$name\"] | .units | .[\"$name/$unit_index\"]"

	echo ".applications | select(($path | .[\"juju-status\"] | .current == \"idle\") and ($path | .[\"workload-status\"] | .current == \"active\")) | keys[$app_index]"
}

idle_subordinate_condition() {
	local name parent unit_index

	name=${1}
	parent=${2}
	unit_index=${3:-0}

	path=".[\"$parent\"] | .units | .[] | .subordinates | .[\"$name/$unit_index\"]"

	# Print the *subordinate* name if it has an idle status in parent application
	echo ".applications | select(($path | .[\"juju-status\"] | .current == \"idle\") and ($path | .[\"workload-status\"] | .current != \"error\")) | \"$name\""
}

active_condition() {
	local name app_index

	name=${1}
	app_index=${2:-0}

	echo ".applications | select(.[\"$name\"] | .[\"application-status\"] | .current == \"active\") | keys[$app_index]"
}

# not_idle_list should be used where you expect an arbitrary list of applications whose agent-status are not in idle state,
# ideally applications in a bundle, this helps the tests to avoid being overly specific to a given number of applications.
# e.g. wait_for 0 "$(not_idle_list) | length" 1800
not_idle_list() {
	echo '[.applications[] | select((.units[] | .["juju-status"].current != "idle") or (.units[] | .["workload-status"].current == "error"))]'
}

# workload_status gets the workload-status object for the unit - use
# .current or .message to select the actual field you need.
workload_status() {
	local app unit

	app=$1
	unit=$2

	echo ".applications[\"$app\"].units[\"$app/$unit\"][\"workload-status\"]"
}

# agent_status gets the juju-status object for the unit - use
# .current or .message to select the actual field you need.
agent_status() {
	local app unit

	app=$1
	unit=$2

	echo ".applications[\"$app\"].units[\"$app/$unit\"][\"juju-status\"]"
}

# charm_rev gets the current juju-status object for the application and uses it
# to find the application charm-rev.
charm_rev() {
	local app rev

	app=$1
	rev=${2:-0}

	echo ".applications | select(.[\"$app\"] | .[\"charm-rev\"] == $rev)"
}

# charm_channel gets the current juju-status object for the application and uses it
# to find the application charm-channel.
charm_channel() {
	local app channel

	app=$1
	channel=$2

	echo ".applications | select(.[\"$app\"] | .[\"charm-channel\"] == \"$channel\")"
}

# wait_for_machine_agent_status blocks until the machine agent for the specified
# machine instance ID reports the requested status.
#
# ```
# wait_for_machine_agent_status <instance-id> <status>
#
# example:
# wait_for_machine_agent_status "i-1234" "started"
# ```
wait_for_machine_agent_status() {
	local inst_id status

	inst_id=${1}
	status=${2}

	attempt=0
	# shellcheck disable=SC2046,SC2143
	until [ $(juju show-machine --format json | jq -r ".[\"machines\"] | .[\"${inst_id}\"] | .[\"juju-status\"] | .[\"current\"]" | grep "${status}") ]; do
		echo "[+] (attempt ${attempt}) polling machines"
		juju machines | grep "$inst_id" 2>&1 | sed 's/^/    | /g'
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))
	done

	if [[ ${attempt} -gt 0 ]]; then
		echo "[+] $(green 'Completed polling machines')"
		juju machines | grep "$inst_id" 2>&1 | sed 's/^/    | /g'
		sleep "${SHORT_TIMEOUT}"
	fi
}

# wait_for_container_agent_status blocks until the machine agent for the specified
# machine instance ID reports the requested status.
#
# ```
# wait_for_container_agent_status <parent-instance-id> <status>
#
# example:
# wait_for_container_agent_status "0/lxd/0 "started"
# ```
wait_for_container_agent_status() {
	local inst_id status

	inst_id=${1}
	status=${2}

	parent_id=$(echo "${inst_id}" | awk 'BEGIN {FS="/";} {print $1}')

	attempt=0
	# shellcheck disable=SC2046,SC2143
	until [ $(juju show-machine --format json | jq -r ".[\"machines\"] | .[\"${parent_id}\"] | .[\"containers\"] | .[\"${inst_id}\"] | .[\"juju-status\"] | .[\"current\"]" | grep "${status}") ]; do
		echo "[+] (attempt ${attempt}) polling machines"
		juju machines | grep "$inst_id" 2>&1 | sed 's/^/    | /g'
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))
	done

	if [[ ${attempt} -gt 0 ]]; then
		echo "[+] $(green 'Completed polling machines')"
		juju machines | grep "$inst_id" 2>&1 | sed 's/^/    | /g'
		sleep "${SHORT_TIMEOUT}"
	fi
}

# wait_for_machine_netif_count blocks until the number of detected network
# interfaces for the requested machine instance ID becomes equal to the desired
# value.
#
# ```
# wait_for_machine_netif_count <instance-id> <count>
#
# example:
# wait_for_machine_netif_count "i-1234" "42"
# ```
wait_for_machine_netif_count() {
	local inst_id count

	inst_id=${1}
	count=${2}

	attempt=0
	# shellcheck disable=SC2046,SC2143
	until [ $(juju show-machine --format json | jq -r ".[\"machines\"] | .[\"${inst_id}\"] | .[\"network-interfaces\"] | length" | grep "${count}") ]; do
		# shellcheck disable=SC2046,SC2143
		echo "[+] (attempt ${attempt}) network interface count for instance ${inst_id} = "$(juju show-machine --format json | jq -r ".[\"machines\"] | .[\"${inst_id}\"] | .[\"network-interfaces\"] | length")
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))
	done

}

# wait_for_subordinate_count blocks until the number of subordinates
# to the desired unit becomes equal to the desired value.
#
# ```
# wait_for_subordinate_count <application name> <principal unit num> <count>
#
# example:
# wait_for_subordinate_count mysql 0 3
# ```
wait_for_subordinate_count() {
	local name unit_index count

	name=${1}
	unit_index=${2:-0}
	count=${3:-0}

	attempt=0
	# shellcheck disable=SC2046,SC2143
	until [ $(juju status --format json | jq -r ".applications | .[\"${name}\"] | .units | .[\"${name}/${unit_index}\"] | .subordinates | length" | grep "${count}") ]; do
		# shellcheck disable=SC2046,SC2143
		echo "[+] (attempt ${attempt}) subordinate count for unit ${name}/${unit_index} = "$(juju status --format json | jq -r ".applications | .[\"${name}\"] | .units | .[\"${name}/${unit_index}\"] | .subordinates  | length")
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))
	done

	if [[ ${attempt} -gt 0 ]]; then
		echo "[+] $(green 'Completed polling status')"
		juju status 2>&1 | sed 's/^/    | /g'
		sleep "${SHORT_TIMEOUT}"
	fi
}

# wait_for_model blocks until a model appears
# interfaces for the requested machine instance ID becomes equal to the desired
# value.
#
# ```
# wait_for_model <name>
#
# example:
# wait_for_model "default"
# ```
wait_for_model() {
	local name

	name=${1}

	attempt=0
	# shellcheck disable=SC2046,SC2143
	until [ $(juju models --format=json | jq -r ".models | .[] | select(.[\"short-name\"] == \"${name}\") | .[\"short-name\"]" | grep "${name}") ]; do
		echo "[+] (attempt ${attempt}) polling models"
		juju models | sed 's/^/    | /g'
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))
	done

	if [[ ${attempt} -gt 0 ]]; then
		echo "[+] $(green 'Completed polling models')"
		juju models | sed 's/^/    | /g'
		sleep "${SHORT_TIMEOUT}"
	fi
}

# wait_for_systemd_service_files_to_appear blocks until the systemd service
# file for a unit is written to disk.
#
# ```
# wait_for_systemd_service_files_to_appear <unit_name>
#
# example:
# wait_for_systemd_service_files_to_appear "ubuntu/0"
# ```
wait_for_systemd_service_files_to_appear() {
	local unit

	unit=${1}
	# shellcheck disable=SC2086
	svc_file_path="/etc/systemd/system/jujud-unit-$(echo -n ${1} | tr '/' '-').service"

	attempt=0
	# shellcheck disable=SC2046,SC2143
	while [ "$attempt" != "3" ]; do
		echo "[+] (attempt ${attempt}) waiting for the systemd unit files for ${unit} to appear"

		svc_present=$(juju ssh "${unit}" "ls ${svc_file_path} 2>/dev/null || echo -n 'missing'")
		if [[ ${svc_present} != "missing" ]]; then
			echo "[+] systemd unit files for ${unit} are now available"
			return
		fi

		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))
	done

	# shellcheck disable=SC2046,SC2005
	echo $(red "Timed out waiting for the systemd unit files for ${unit} to appear")
	exit 1
}

# wait_for_storage is like wait_for but for storage formats. Used to wait for a certain condition in charm storage.
wait_for_storage() {
	local name query timeout

	name=${1}
	query=${2}
	timeout=${3:-600} # default timeout: 600s = 10m

	attempt=0
	start_time="$(date -u +%s)"
	# shellcheck disable=SC2046,SC2143
	until [[ "$(juju storage --format=json 2>/dev/null | jq "${query}" | grep "${name}")" ]]; do
		echo "[+] (attempt ${attempt}) polling status for" "${query} => ${name}"
		juju storage 2>&1 | sed 's/^/    | /g'
		sleep "${SHORT_TIMEOUT}"

		elapsed=$(date -u +%s)-$start_time
		if [[ ${elapsed} -ge ${timeout} ]]; then
			echo "[-] $(red 'timed out waiting for')" "$(red "${name}")"
			exit 1
		fi

		attempt=$((attempt + 1))
	done

	if [[ ${attempt} -gt 0 ]]; then
		echo "[+] $(green 'Completed polling status for')" "$(green "${name}")"
		juju storage 2>&1 | sed 's/^/    | /g'
		# Although juju reports as an idle condition, some charms require a
		# breathe period to ensure things have actually settled.
		sleep "${SHORT_TIMEOUT}"
	fi
}

# wait_for_aws_ingress_cidrs_for_port_range blocks until the expected CIDRs
# are present in the AWS security group rules for the specified port range.
wait_for_aws_ingress_cidrs_for_port_range() {
	local from_port to_port exp_cidrs cidr_type

	from_port=${1}
	to_port=${2}
	exp_cidrs=${3}
	cidr_type=${4}

	ipV6Suffix=""
	if [ "$cidr_type" = "ipv6" ]; then
		ipV6Suffix="v6"
	fi

	# shellcheck disable=SC2086
	secgrp_list=$(aws ec2 describe-security-groups --filters Name=ip-permission.from-port,Values=${from_port} Name=ip-permission.to-port,Values=${to_port})
	# print the security group rules
	# shellcheck disable=SC2086
	got_cidrs=$(echo ${secgrp_list} | jq -r ".SecurityGroups[0].IpPermissions // [] | .[] | select(.FromPort == ${from_port} and .ToPort == ${to_port}) | .Ip${ipV6Suffix}Ranges // [] | .[] | .CidrIp${ipV6Suffix}" | sort | paste -sd, -)

	attempt=0
	# shellcheck disable=SC2046,SC2143
	while [ "$attempt" -lt "3" ]; do
		echo "[+] (attempt ${attempt}) polling security group rules"
		# shellcheck disable=SC2086
		secgrp_list=$(aws ec2 describe-security-groups --filters Name=ip-permission.from-port,Values=${from_port} Name=ip-permission.to-port,Values=${to_port})
		# shellcheck disable=SC2086
		got_cidrs=$(echo ${secgrp_list} | jq -r ".SecurityGroups[0].IpPermissions // [] | .[] | select(.FromPort == ${from_port} and .ToPort == ${to_port}) | .Ip${ipV6Suffix}Ranges // [] | .[] | .CidrIp${ipV6Suffix}" | sort | paste -sd, -)
		sleep "${SHORT_TIMEOUT}"

		if [ "$got_cidrs" == "$exp_cidrs" ]; then
			break
		fi

		attempt=$((attempt + 1))
	done

	if [ "$got_cidrs" != "$exp_cidrs" ]; then
		# shellcheck disable=SC2046
		echo $(red "expected generated EC2 ${cidr_type} ingress CIDRs for range [${from_port}, ${to_port}] to be:\n${exp_cidrs}\nGOT:\n${got_cidrs}")
		exit 1
	fi

	echo "[+] security group rules for port range [${from_port}, ${to_port}] and CIDRs ${exp_cidrs} updated"
}
