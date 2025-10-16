# add_multi_nic_machine()
#
# Create a new machine, wait for it to boot and hotplug a pre-allocated
# network interface which has been tagged: "nic-type: hotpluggable".
# Then restart the machine agent and wait for juju to detect 2 network
# interfaces before returning.
#
# NICs are automatically setup on Ubuntu 24.04+ by cloud-init's hotplug module.
# See https://repost.aws/knowledge-center/ec2-ubuntu-secondary-network-interface
# as a reference. If provisioning older Ubuntu releases, additional steps
# with netplan are required.
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

	echo "[+] restarting machine agent on ${juju_machine_id}..."
	juju ssh "${juju_machine_id}" 'sudo systemctl restart jujud-machine-*'

	# Wait for the interface to be detected by juju
	echo "[+] waiting for juju to detect added NIC"
	wait_for_machine_netif_count "$juju_machine_id" "2"
}
