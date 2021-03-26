test_expose_ec2() {
	if [ "$(skip 'test_expose_ec2')" ]; then
		echo "==> TEST SKIPPED: expose application tests (EC2)"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju aws

	file="${TEST_DIR}/test-expose-ec2.log"

	bootstrap "test-expose-ec2" "${file}"

	test_expose_app_ec2
	test_bundle_with_exposed_endpoints

	destroy_controller "test-expose-ec2"
}
