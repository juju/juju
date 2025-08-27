test_firewall() {
	if [ "$(skip 'test_firewall')" ]; then
		echo "==> TEST SKIPPED: firewall tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	case "${BOOTSTRAP_PROVIDER:-}" in
	"ec2")
		setup_awscli_credential
		check_dependencies juju aws
		;;
	"gce")
		setup_gcloudcli_credential
		check_dependencies juju gcloud
		;;
	*)
		check_dependencies juju
		;;
	esac

	file="${TEST_DIR}/test-firewall.txt"

	bootstrap "test-firewall" "${file}"

	case "${BOOTSTRAP_PROVIDER:-}" in
	"ec2")
		test_firewall_ssh_ec2
		;;
	"gce")
		test_firewall_ssh_gce
		;;
	*)
		echo "==> TEST SKIPPED: test_firewall_ssh test runs on aws or gce only"
		;;
	esac

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
