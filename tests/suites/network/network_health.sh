run_network_health() {
	echo

	file="${TEST_DIR}/network-health.txt"

	ensure "network-health" "${file}"

	# Deploy some applications for different series.
	juju deploy mongodb --series focal
	juju deploy juju-gui --series xenial
	juju deploy ubuntu ubuntu-bionic --series bionic

	# Now the testing charm for each series.
	juju deploy 'cs:~juju-qa/network-health' network-health-xenial --series xenial
	juju deploy 'cs:~juju-qa/network-health' network-health-bionic --series bionic
	juju deploy 'cs:~juju-qa/network-health' network-health-focal --series focal

	juju expose network-health-xenial
	juju expose network-health-bionic
	juju expose network-health-focal

	juju add-relation network-health-focal mongodb
	juju add-relation network-health-xenial juju-gui
	juju add-relation network-health-bionic ubuntu-bionic

	wait_for "mongodb" "$(idle_condition "mongodb" 1)"
	wait_for "juju-gui" "$(idle_condition "juju-gui" 0)"
	wait_for "ubuntu-bionic" "$(idle_condition "ubuntu-bionic" 5)"
	wait_for "network-health-focal" "$(idle_subordinate_condition "network-health-focal" "mongodb")"
	wait_for "network-health-bionic" "$(idle_subordinate_condition "network-health-bionic" "ubuntu-bionic")"
	wait_for "network-health-xenial" "$(idle_subordinate_condition "network-health-xenial" "juju-gui")"

	check_default_routes
	check_accessibility

	destroy_model "network-health"
}

check_default_routes() {
	echo "[+] checking default routes"

	for machine in $(juju machines --format=json | jq -r ".machines | keys | .[]"); do
		default=$(juju run --machine "$machine" -- ip route show | grep default)
		if [ -z "$default" ]; then
			echo "No default route detected for machine ${machine}"
			exit 1
		fi
	done
}

check_accessibility() {
	echo "[+] checking neighbour connectivity and external access"

	for net_health_unit in "network-health-xenial/0" "network-health-bionic/0" "network-health-focal/0"; do
		ip="$(juju show-unit $net_health_unit --format json | jq -r ".[\"$net_health_unit\"] | .[\"public-address\"]")"

		curl_cmd="curl 2>/dev/null ${ip}:8039"

		# Check that each of the principles can access the subordinate.
		for principle_unit in "mongodb/0" "juju-gui/0" "ubuntu-bionic/0"; do
			check_contains "$(juju run --unit $principle_unit "$curl_cmd")" "pass"
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
