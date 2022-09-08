test_cli() {
	if [ "$(skip 'test_cli')" ]; then
		echo "==> TEST SKIPPED: CLI tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-cli.log"

	bootstrap "test-cli" "${file}"

	test_display_clouds
	test_local_charms
	test_model_config
	test_model_defaults

	destroy_controller "test-cli"
}
