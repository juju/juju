test_appdata() {
	if [ "$(skip 'test_appdata')" ]; then
		echo "==> TEST SKIPPED: appdata tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-appdata.log"

	bootstrap "test-appdata" "${file}"

	test_appdata_int

	destroy_controller "test-appdata"
}
