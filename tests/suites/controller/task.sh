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
	# Intentionally left failing: HA scale-out for the snap-based
	# standalone controller is Phase 2 (JUJU-9740) and is not implemented.
	# The failure keeps the gap visible; see enable_ha.sh.
	test_enable_ha

	destroy_controller "test-controller"
}
