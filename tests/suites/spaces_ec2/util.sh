# add_multi_nic_machine()
#
# Create a new machine, wait for it to boot and hotplug a pre-allocated
# network interface which has been tagged: "nic-type: hotpluggable".
#
# Then, patch the netplan settings for the new interface, apply the new plan,
# restart the machine agent and wait for juju to detect the new interface
# before returning.
add_multi_nic_machine() {
	hotplug_nic_id=$1

	juju add-machine
	juju_machine_id=$(juju show-machine --format json | jq -r '.["machines"] | keys[0]')
	echo "[+] waiting for machine ${juju_machine_id} to start..."

	wait_for_machine_agent_status "$juju_machine_id" "started"

	# Hotplug the second network device to the machine
	echo "[+] hotplugging second NIC with ID ${hotplug_nic_id} to machine ${juju_machine_id}..."
	# shellcheck disable=SC2046,SC2086
	aws ec2 attach-network-interface --device-index 1 \
		--network-interface-id ${hotplug_nic_id} \
		--instance-id $(juju show-machine --format json | jq -r ".[\"machines\"] | .[\"${juju_machine_id}\"] | .[\"instance-id\"]")

	# Add an entry to netplan and apply it so the second interface comes online
	echo "[+] updating netplan and restarting machine agent"
	# shellcheck disable=SC2086,SC2016
	juju ssh ${juju_machine_id} 'sudo sh -c "echo \"            gateway4: `ip route | grep default | cut -d\" \" -f3`\n        ens6:\n            dhcp4: true\n\" >> /etc/netplan/50-cloud-init.yaml"'
	# shellcheck disable=SC2086,SC2016
	juju ssh ${juju_machine_id} 'sudo netplan apply'
	# shellcheck disable=SC2086,SC2016
	juju ssh ${juju_machine_id} 'sudo systemctl restart jujud-machine-*'

	# Wait for the interface to be detected by juju
	echo "[+] waiting for juju to detect added NIC"
	wait_for_machine_netif_count "$juju_machine_id" "3"
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
	got_if=$(juju run -a ${app_name} "network-get ${endpoint_name}" | grep "interfacename: ens" | awk '{print $2}')
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
	got=$(juju show-application ${app_name} --format json | jq -r ".[\"${app_name}\"] | .[\"endpoint-bindings\"] | .[\"${endpoint_name}\"]")
	if [ "$got" != "$exp_space_name" ]; then
		# shellcheck disable=SC2086,SC2016,SC2046
		echo $(red "Expected endpoint \"${endpoint_name}\" in juju show-application ${app_name} to be ${exp_space_name}; got ${got}")
		exit 1
	fi
}

# get_unit_index(app_name)
#
# Lookup and return the unit index for app_name.
get_unit_index() {
	local app_name

	app_name=${1}

	index=$(juju status | grep "${app_name}/" | cut -d' ' -f1 | cut -d'/' -f2 | cut -d'*' -f1)
	echo "$index"
}
