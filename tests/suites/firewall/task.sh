test_firewall() {
	if [ "$(skip 'test_firewall')" ]; then
		echo "==> TEST SKIPPED: firewall tests"
		return
	fi

	set_verbosity

	setup_awscli_credential

	echo "==> Checking for dependencies"
	check_dependencies juju aws

	file="${TEST_DIR}/test-firewall.txt"

	bootstrap "test-firewall" "${file}"

	case "${BOOTSTRAP_PROVIDER:-}" in
	"ec2")
		test_expose_app_ec2
		;;
	*)
		echo "==> TEST SKIPPED: test_expose_app_ec2 test runs on aws only"
		;;
	esac

	case "${BOOTSTRAP_PROVIDER:-}" in
	"ec2")
		test_bundle_with_exposed_endpoints
		;;
	*)
		echo "==> TEST SKIPPED: test_bundle_with_exposed_endpoints test runs on aws only"
		;;
	esac

	destroy_controller "test-firewall"
}
