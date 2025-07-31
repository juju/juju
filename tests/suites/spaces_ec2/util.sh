# add_multi_nic_machine()
#
# Create a new machine, wait for it to boot and hotplug a pre-allocated
# network interface which has been tagged: "nic-type: hotpluggable".
add_multi_nic_machine() {
	local hotplug_nic_id
	hotplug_nic_id=$1

	# Ensure machine is deployed to the same az as our nic
	az=$(aws ec2 describe-network-interfaces --filters Name=network-interface-id,Values="$hotplug_nic_id" | jq -r ".NetworkInterfaces[0].AvailabilityZone")
	juju add-machine --constraints zones="${az}"
	juju_machine_id=$(juju show-machine --format json | jq -r '.["machines"] | keys[0]')
	echo "[+] waiting for machine ${juju_machine_id} to start..."

	wait_for_machine_agent_status "$juju_machine_id" "started"

	# Hotplug the second network device to the machine
	echo "[+] hotplugging second NIC with ID ${hotplug_nic_id} to machine ${juju_machine_id}..."
	# shellcheck disable=SC2046,SC2086
	aws ec2 attach-network-interface --device-index 1 \
		--network-interface-id ${hotplug_nic_id} \
		--instance-id $(juju show-machine --format json | jq -r ".[\"machines\"] | .[\"${juju_machine_id}\"] | .[\"instance-id\"]")

	# Wait until the new NIC is UP
	timeout=${3:-600} # default timeout: 600s = 10m
	start_time="$(date -u +%s)"
	while true; do
		if juju ssh ${juju_machine_id} 'test $(ls /sys/class/net | grep "ens\|enp\|eth" | wc -l) -eq 2 && echo done' | grep "done"; then
			echo "[+] second NIC attached."
			break
		fi

		elapsed=$(date -u +%s)-$start_time
		if [[ ${elapsed} -ge ${timeout} ]]; then
			echo "[-] $(red 'timed out waiting for new NIC')"
			exit 1
		fi

		sleep 1
	done
}

# configure_multi_mic_netplan()
#
# Patch the netplan settings for the new interface, apply the new plan,
# restart the machine agent and wait for juju to detect the new interface
# before returning.
configure_multi_nic_netplan() {
	local juju_machine_id hotplug_iface
	juju_machine_id=$1
	hotplug_iface=$2

	juju ssh "${juju_machine_id}" "sudo apt install yq -y"

	# Add an entry to netplan and apply it so the second interface comes online
	echo "[+] updating netplan and restarting machine agent"

	add_routes_yq='.network.ethernets[].routes = [{\"to\": \"default\", \"via\": \"$(ip route | grep default | cut -d " " -f3)\"}]'
	add_routes_cmd="sudo yq -i -y \"${add_routes_yq}\" /etc/netplan/50-cloud-init.yaml"

	add_dhcp4_eth_yq=".network.ethernets.${hotplug_iface}.dhcp4 = true"
	add_dhcp4_eth_cmd="sudo yq -i -y \"${add_dhcp4_eth_yq}\" /etc/netplan/50-cloud-init.yaml"

	juju ssh ${juju_machine_id} "${add_routes_cmd}"
	juju ssh ${juju_machine_id} "${add_dhcp4_eth_cmd}"

	echo "[+] Reconfiguring netplan:"
	juju ssh ${juju_machine_id} 'sudo cat /etc/netplan/50-cloud-init.yaml'
	juju ssh ${juju_machine_id} 'sudo netplan apply' || true

	echo "[+] Applied"
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

# get_unit_index(app_name)
#
# Lookup and return the unit index for app_name.
get_unit_index() {
	local app_name

	app_name=${1}

	index=$(juju status | grep "${app_name}/" | cut -d' ' -f1 | cut -d'/' -f2 | cut -d'*' -f1)
	echo "$index"
}
