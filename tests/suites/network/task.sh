test_network() {
	if [ "$(skip 'test_network')" ]; then
		echo "==> TEST SKIPPED: network tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-network.txt"

	bootstrap "test-network" "${file}"

	test_network_health

	destroy_controller "test-network"
}
