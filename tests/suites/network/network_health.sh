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

	wait_for "ubuntu-focal" "$(idle_condition "ubuntu-focal")" 1800
	wait_for "ubuntu-jammy" "$(idle_condition "ubuntu-jammy")"

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

test_network_health() {
	if [ "$(skip 'test_network_health')" ]; then
		echo "==> TEST SKIPPED: test_network_health"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_network_health"
	)
}
