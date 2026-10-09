test_controller() {
	if [ "$(skip 'test_controller')" ]; then
		echo "==> TEST SKIPPED: controller tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	case "${BOOTSTRAP_PROVIDER:-}" in
	"ec2")
		setup_awscli_credential
		;;
	"gce")
		setup_gcloudcli_credential
		;;
	esac

	file="${TEST_DIR}/test-controller.log"

	bootstrap "test-controller" "${file}"

	test_metrics

	test_query_tracing
	test_limit_access
	#test_limit_access_ha
	test_enable_ha

	destroy_controller "test-controller"
}
