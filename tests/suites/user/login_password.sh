# run_user_change_password changes the 'admin' user password, logout and tries to login with new password
run_user_change_password() {
	# Echo out to ensure nice output to the test suite.
	echo

	# The following ensures that a bootstrap juju exists.
	file="${TEST_DIR}/test-user-change-password.log"
	ensure "user-change-password" "${file}"

	echo "Change admin password"
	expect_that "juju change-user-password" "
expect \"new password: \" {
	send \"test-password\r\"
	expect \"type new password again: \" {
		send \"test-password\r\"
	}
}"

	echo "Logout from controller"
	juju logout

	echo "Login as admin"
	expect_that "juju login" "
expect \"Enter username: \" {
	send \"admin\r\"
	expect \"please enter password for admin*\" {
		send \"test-password\r\"
	}
}" 15

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
