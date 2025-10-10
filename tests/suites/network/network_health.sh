run_network_health() {
	echo

	file="${TEST_DIR}/network-health.txt"

	ensure "network-health" "${file}"

	# Deploy some applications for different series.
	juju deploy ubuntu ubuntu-focal --base ubuntu@20.04
	juju deploy ubuntu ubuntu-jammy --base ubuntu@22.04

	# Now the testing charm for each series.

	juju deploy 'juju-qa-network-health' network-health-focal --base ubuntu@20.04
	juju deploy 'juju-qa-network-health' network-health-jammy --base ubuntu@22.04

	juju integrate network-health-focal ubuntu-focal
	juju integrate network-health-jammy ubuntu-jammy

	juju expose network-health-focal
	juju expose network-health-jammy

	wait_for "ubuntu-focal" "$(idle_condition "ubuntu-focal" 2)" 1800
	wait_for "ubuntu-jammy" "$(idle_condition "ubuntu-jammy" 3)"

	wait_for "network-health-focal" "$(idle_subordinate_condition "network-health-focal" "ubuntu-focal")"
	wait_for "network-health-jammy" "$(idle_subordinate_condition "network-health-jammy" "ubuntu-jammy")"

	check_default_routes
	check_accessibility

	destroy_model "network-health"
}

check_default_routes() {
	echo "[+] checking default routes"

	for machine in $(juju machines --format=json | jq -r ".machines | keys | .[]"); do
		default=$(juju exec --machine "$machine" -- ip route show | grep default)
		if [ -z "$default" ]; then
			echo "No default route detected for machine ${machine}"
			exit 1
		fi
	done
}

check_accessibility() {
	echo "[+] checking neighbour connectivity and external access"

	for net_health_unit in "network-health-focal/0" "network-health-jammy/0"; do

		ip="$(juju show-unit $net_health_unit --format json | jq -r ".[\"$net_health_unit\"] | .[\"public-address\"]")"

		curl_cmd="curl -s http://${ip}:8039"

		# Check that each of the principles can access the subordinate.
		for principle_unit in "ubuntu-focal/0" "ubuntu-jammy/0"; do
			echo "checking network health unit ${net_health_unit} reachability from ${principle_unit} using ${ip}:8039"
			check_contains "$(juju exec --unit $principle_unit "$curl_cmd")" "pass"
		done

		echo "checking network health unit ${net_health_unit} reachability from host using ${ip}:8039"
		# Check that the exposed subordinate is accessible externally.
		check_contains "$($curl_cmd)" "pass"
	done
}

run_ip_address_change() {
	echo

	file="${TEST_DIR}/ip-address-change.txt"

	ensure "ip-address-change" "${file}"
	juju switch "ip-address-change"
	juju deploy juju-qa-test -n 2

	wait_for "juju-qa-test" "$(active_condition "juju-qa-test" 0)"

	instance_0="$(juju show-machine 0 --format json | jq '.machines["0"] | .["instance-id"]' -r)"
	instance_1="$(juju show-machine 1 --format json | jq '.machines["1"] | .["instance-id"]' -r)"

	old_ip_instance_0="$(lxc exec "${instance_0}" -- hostname -i)"
	old_ip_instance_1="$(lxc exec "${instance_1}" -- hostname -i)"

	# Trigger an IP address change for machine 0.
	lxc config device add "${instance_0}" eth0 none
	sleep 5
	lxc config device remove "${instance_0}" eth0
	new_ip_instance_0="$(lxc exec "${instance_0}" -- hostname -i)"

	# Check that the IP address are the same when running from lxc and juju.
	new_ip_instance_0_from_jujuexec=""
	attempt=0
	echo "getting the new ip address for machine-0"
	while true; do
		new_ip_instance_0_from_jujuexec=$(timeout 5s juju exec --unit juju-qa-test/0 -- hostname -I || true)
		if echo "${new_ip_instance_0_from_jujuexec}" | grep -qF "${new_ip_instance_0}"; then
			# shellcheck disable=SC2046
			echo $(green "ip address for machine-0 matches: ${new_ip_instance_0_from_jujuexec}")
			break
		fi

		attempt=$((attempt + 1))
		if [ $attempt -eq 30 ]; then
			# shellcheck disable=SC2046
			echo $(red "timeout: waiting for machine-0 ip change to ${new_ip_instance_0}")
			exit 1
		fi
		sleep 5
	done

	check_not_contains "${new_ip_instance_0_from_jujuexec}" "${old_ip_instance_0}"

	# Trigger an IP address change for machine 1.
	lxc config device add "${instance_1}" eth0 none
	sleep 5
	lxc config device remove "${instance_1}" eth0
	new_ip_instance_1="$(lxc exec "${instance_1}" -- hostname -i)"

	# Check that the IP address are the same when running from lxc and juju.
	new_ip_instance_1_from_jujuexec=""
	attempt=0
	echo "getting the new ip address for machine-1"
	while true; do
		new_ip_instance_1_from_jujuexec=$(timeout 5s juju exec --unit juju-qa-test/1 -- hostname -I || true)
		if echo "${new_ip_instance_1_from_jujuexec}" | grep -qF "${new_ip_instance_1}"; then
			# shellcheck disable=SC2046
			echo $(green "ip address for machine-1 matches: ${new_ip_instance_1_from_jujuexec}")
			break
		fi

		attempt=$((attempt + 1))
		if [ $attempt -eq 30 ]; then
			# shellcheck disable=SC2046
			echo $(red "timeout: waiting for machine-1 ip change to ${new_ip_instance_1}")
			exit 1
		fi
		sleep 5
	done

	check_not_contains "${new_ip_instance_1_from_jujuexec}" "${old_ip_instance_1}"

	destroy_model "ip-address-change"
}

test_network_health() {
	if [ "$(skip 'test_network_health')" ]; then
		echo "==> TEST SKIPPED: test_network_health"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_network_health"

		if [ "${BOOTSTRAP_PROVIDER}" = "lxd" ]; then
			run "run_ip_address_change"
		else
			echo "==> TEST SKIPPED: run_ip_address_change - tests for LXD only"
		fi
	)
}
