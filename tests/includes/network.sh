assert_machine_ip_is_in_cidrs() {
	local machine_index cidrs

	machine_index=${1}
	cidrs=${2}

	if ! which "grepcidr" >/dev/null 2>&1; then
		sudo apt install grepcidr -y
	fi

	for cidr in $cidrs; do
		machine_ip_in_cidr=$(juju machines --format json | yq -r ".machines[\"${machine_index}\"][\"ip-addresses\"][]" | grepcidr "${cidr}" || echo "")
		if [ -n "${machine_ip_in_cidr}" ]; then
			echo "${machine_ip_in_cidr}"
			return
		fi
	done

	# shellcheck disable=SC2086,SC2016,SC2046
	echo $(red "machine ${machine_index} has no ips in subnet ${cidrs}") 1>&2
	exit 1
}

# assert_net_iface_for_endpoint_matches(app_name, endpoint_name, exp_if_name)
#
# Verify that the (non-fan) network adapter assigned to the specified endpoint
# matches the provided value.
assert_net_iface_for_endpoint_matches() {
	local app_name endpoint_name exp_if_name

	app_name=${1}
	endpoint_name=${2}
	exp_if_name=${3}

	# shellcheck disable=SC2086,SC2016
	got_if=$(juju_exec_output -a ${app_name} "network-get ${endpoint_name}" | grep "interfacename: en" | awk '{print $2}' || echo "")
	if [ "$got_if" != "$exp_if_name" ]; then
		# shellcheck disable=SC2086,SC2016,SC2046
		echo $(red "Expected network interface for ${app_name}:${endpoint_name} to be ${exp_if_name}; got ${got_if}")
		exit 1
	fi
}

# assert_endpoint_binding_matches(app_name, endpoint_name, exp_space_name)
#
# Verify that show-application shows that the specified endpoint is bound to
# the provided space name.
assert_endpoint_binding_matches() {
	local app_name endpoint_name exp_space_name

	app_name=${1}
	endpoint_name=${2}
	exp_space_name=${3}

	# shellcheck disable=SC2086,SC2016
	got=$(juju show-application ${app_name} --format json | yq -r ".[\"${app_name}\"] | .[\"endpoint-bindings\"] | .[\"${endpoint_name}\"]" || echo "")
	if [ "$got" != "$exp_space_name" ]; then
		# shellcheck disable=SC2086,SC2016,SC2046
		echo $(red "Expected endpoint ${endpoint_name} in juju show-application ${app_name} to be ${exp_space_name}; got ${got}")
		exit 1
	fi
}

# assert_opened_ports_output checks that the open-port and opened-ports hook
# tools behave as expected on the given application unit: it opens a port
# range for all endpoints, then — when both <endpoint> and
# <endpoint_port_range> are supplied — opens a port for that specific
# endpoint. It asserts both the legacy and the --endpoints output formats.
# Ports are ordered by start port in the output, so expected values are
# sorted accordingly. Both port ranges must use the same protocol; this
# helper does not sort or validate across mixed TCP/UDP ranges.
#
# Usage:
#   assert_opened_ports_output <app_name> <all_endpoints_port_range> \
#       [<endpoint> <endpoint_port_range>]
assert_opened_ports_output() {
	local app_name all_ports endpoint endpoint_ports
	local exp_legacy exp_endpoints all_start ep_start

	if [ "$#" -lt 2 ] || [ "$#" -gt 4 ]; then
		echo "ERROR: usage: assert_opened_ports_output <app> <all-ports> [<endpoint> <endpoint-ports>]" >&2
		return 1
	fi

	app_name=${1:-}
	all_ports=${2:-}
	endpoint=${3:-}
	endpoint_ports=${4:-}
	if [ -z "${app_name}" ] || [ -z "${all_ports}" ]; then
		echo "ERROR: application and all-endpoints port range are required" >&2
		return 1
	fi
	if { [ -n "${endpoint}" ] && [ -z "${endpoint_ports}" ]; } || \
		{ [ -z "${endpoint}" ] && [ -n "${endpoint_ports}" ]; }; then
		echo "ERROR: endpoint and endpoint port range must be supplied together" >&2
		return 1
	fi

	# Reject mixed protocols: the current sorter only compares numeric start
	# ports, which is valid only when both ranges share the same protocol.
	if [ -n "${endpoint_ports}" ]; then
		if [ "${all_ports##*/}" != "${endpoint_ports##*/}" ]; then
			echo "ERROR: this helper only accepts ranges with the same protocol: ${all_ports} vs ${endpoint_ports}" >&2
			return 1
		fi
	fi

	all_start="${all_ports%%[-\/]*}"
	if ! [[ ${all_start} =~ ^[0-9]+$ ]]; then
		echo "ERROR: invalid all-endpoints port range: ${all_ports}" >&2
		return 1
	fi
	if [ -n "${endpoint_ports}" ]; then
		ep_start="${endpoint_ports%%[-\/]*}"
		if ! [[ ${ep_start} =~ ^[0-9]+$ ]]; then
			echo "ERROR: invalid endpoint port range: ${endpoint_ports}" >&2
			return 1
		fi
	fi

	echo "==> Checking open/opened-ports hook tools work as expected"

	juju exec --unit "${app_name}/0" "open-port ${all_ports}"

	if [ -n "${endpoint}" ]; then
		juju exec --unit "${app_name}/0" "open-port ${endpoint_ports} --endpoints ${endpoint}"
	fi

	# Build expected outputs sorted by numeric start port, matching the
	# order produced by opened-ports (sorts by protocol, then by port).
	if [ -n "${endpoint_ports}" ]; then
		ep_start="${endpoint_ports%%[-\/]*}"
		all_start="${all_ports%%[-\/]*}"
		if [ "${ep_start}" -lt "${all_start}" ]; then
			exp_legacy="${endpoint_ports} ${all_ports}"
			exp_endpoints="${endpoint_ports} (${endpoint}) ${all_ports} (*)"
		else
			exp_legacy="${all_ports} ${endpoint_ports}"
			exp_endpoints="${all_ports} (*) ${endpoint_ports} (${endpoint})"
		fi
	else
		exp_legacy="${all_ports}"
		exp_endpoints="${all_ports} (*)"
	fi

	got=$(juju_exec_output --unit "${app_name}/0" "opened-ports" | tr '\n' ' ' | sed -e 's/[[:space:]]*$//')
	if [ "$got" != "$exp_legacy" ]; then
		# shellcheck disable=SC2046
		echo $(red "expected opened-ports output to be:\n${exp_legacy}\nGOT:\n${got}")
		exit 1
	fi

	got=$(juju_exec_output --unit "${app_name}/0" "opened-ports --endpoints" | tr '\n' ' ' | sed -e 's/[[:space:]]*$//')
	if [ "$got" != "$exp_endpoints" ]; then
		# shellcheck disable=SC2046
		echo $(red "expected opened-ports output when using --endpoints to be:\n${exp_endpoints}\nGOT:\n${got}")
		exit 1
	fi
}
