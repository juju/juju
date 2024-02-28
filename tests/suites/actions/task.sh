test_actions() {
	if [ "$(skip 'test_actions')" ]; then
		echo "==> TEST SKIPPED: actions tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-actions.log"

	bootstrap "actions-test-ctl" "${file}"

	test_actions_params

	destroy_controller "actions-test-ctl"
}
