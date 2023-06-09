test_metrics() {
	if [ "$(skip 'test_metrics')" ]; then
		echo "==> TEST SKIPPED: metrics tests"
		return
	fi

	set_verbosity

	cd .. || exit

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-metrics.log"

	bootstrap "test-metrics" "${file}"

	run "run_smoke_test"

	destroy_controller "test-metrics"
}
