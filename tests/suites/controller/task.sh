test_controller() {
	if [ "$(skip 'test_controller')" ]; then
		echo "==> TEST SKIPPED: controller tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-controller.log"

	bootstrap "test-controller" "${file}"

	test_metrics

	test_enable_ha
	test_query_tracing

	destroy_controller "test-controller"
}
