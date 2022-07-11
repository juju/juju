run_network_health() {
	echo

	file="${TEST_DIR}/network-health.txt"

	ensure "network-health" "${file}"

	# Deploy some applications for different series.
	juju deploy ubuntu ubuntu-focal --series focal
	juju deploy ubuntu ubuntu-jammy --series jammy

	# Now the testing charm for each series.
	juju deploy 'juju-qa-network-health' network-health-focal --series focal
	juju deploy 'juju-qa-network-health' network-health-jammy --series jammy

	juju add-relation network-health-focal ubuntu-focal
	juju add-relation network-health-jammy ubuntu-jammy

	juju expose network-health-focal
	juju expose network-health-jammy

	wait_for "ubuntu-focal" "$(idle_condition "ubuntu-focal" 3)"
	wait_for "ubuntu-jammy" "$(idle_condition "ubuntu-jammy" 4)"
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

	for net_health_unit in "network-health-jammy/0" "network-health-focal/0"; do
		ip="$(juju show-unit $net_health_unit --format json | jq -r ".[\"$net_health_unit\"] | .[\"public-address\"]")"

		curl_cmd="curl 2>/dev/null ${ip}:8039"

		# Check that each of the principles can access the subordinate.
		for principle_unit in "ubuntu-focal/0" "ubuntu-jammy/0"; do
			check_contains "$(juju exec --unit $principle_unit "$curl_cmd")" "pass"
		done

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
