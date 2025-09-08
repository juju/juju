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
