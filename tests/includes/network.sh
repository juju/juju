assert_machine_ip_is_in_cidrs() {
	local machine_index cidrs

	machine_index=${1}
	cidrs=${2}

	if ! which "grepcidr" >/dev/null 2>&1; then
		sudo apt install grepcidr -y
	fi

	for cidr in $cidrs; do
		machine_ip_in_cidr=$(juju machines --format json | jq -r ".machines[\"${machine_index}\"][\"ip-addresses\"][]" | grepcidr "${cidr}" || echo "")
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
	got_if=$(juju exec -a ${app_name} "network-get ${endpoint_name}" | grep "interfacename: en" | awk '{print $2}' || echo "")
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
	got=$(juju show-application ${app_name} --format json | jq -r ".[\"${app_name}\"] | .[\"endpoint-bindings\"] | .[\"${endpoint_name}\"]" || echo "")
	if [ "$got" != "$exp_space_name" ]; then
		# shellcheck disable=SC2086,SC2016,SC2046
		echo $(red "Expected endpoint ${endpoint_name} in juju show-application ${app_name} to be ${exp_space_name}; got ${got}")
		exit 1
	fi
}
