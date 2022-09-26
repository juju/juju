test_user() {
	if [ "$(skip 'test_user')" ]; then
		echo "==> TEST SKIPPED: user tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-user.log"

	bootstrap "test-user" "${file}"

	# Tests that need to be run are added here.
	test_user_manage
	test_user_login_password

	destroy_controller "test-user"
}
