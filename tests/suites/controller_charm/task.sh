test_controller_charm() {
	if [ "$(skip 'test_controller_charm')" ]; then
		echo "==> TEST SKIPPED: controller charm tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-ctr-charm.log"

	bootstrap "test-ctr-charm" "${file}"

	wait_for "controller" "$(idle_condition "controller")"
	test_website
	test_dashboard
	# TODO(barrettj12): add tests for integration with prometheus/loki

	destroy_controller "test-ctr-charm"
}
