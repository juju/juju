test_smoke() {
	if [ "$(skip 'test_smoke')" ]; then
		echo "==> TEST SKIPPED: smoke tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-smoke.log"

	test_build

	bootstrap "test-smoke" "${file}"

	test_deploy

	destroy_controller "test-smoke"
}
