# run_user_change_password changes the 'admin' user password, logout and tries to login with new password
run_user_change_password() {
	# Echo out to ensure nice output to the test suite.
	echo

	# The following ensures that a bootstrap juju exists.
	file="${TEST_DIR}/test-user-change-password.log"
	ensure "user-change-password" "${file}"

	echo "Add test-change-password-user"
	juju add-user test-change-password-user

	echo "Change test-change-password-user password"
	echo "test-password" | juju change-user-password test-change-password-user --no-prompt

	echo "Change admin password"
	echo "admin-password" | juju change-user-password admin --no-prompt

	echo "Logout"
	juju logout

	echo "Login as test-change-password-user"
	echo "test-password" | juju login --user test-change-password-user --no-prompt

	echo "Logout"
	juju logout

	echo "Login as admin"
	echo "admin-password" | juju login --user admin --no-prompt

	destroy_model "user-change-password"
}

test_user_login_password() {
	if [ -n "$(skip 'test_user_login_password')" ]; then
		echo "==> SKIP: Asked to skip user login/password tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_user_change_password"
	)
}
