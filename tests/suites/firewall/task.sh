test_firewall() {
	if [ "$(skip 'test_firewall')" ]; then
		echo "==> TEST SKIPPED: firewall tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju aws

	file="${TEST_DIR}/test-firewall.log"

	bootstrap "test-firewall" "${file}"

	test_expose_app_ec2
	test_bundle_with_exposed_endpoints

	destroy_controller "test-firewall"
}
