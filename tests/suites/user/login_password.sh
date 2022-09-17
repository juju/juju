# ....
run_user_change_password() {
	# Echo out to ensure nice output to the test suite.
	echo

	# The following ensures that a bootstrap juju exists.
	file="${TEST_DIR}/test-user-change-password.log"
	ensure "user-change-password" "${file}"

	echo "Add test-user"
	juju add-user test-user

	echo "Change test-user password"
	expect -c "
		proc abort { } { send_user \"\rTimeout Error!\" ; exit 2 }
		expect_before timeout abort

		set timeout 10
		spawn juju change-user-password test-user
		match_max 100000
		expect \"old password: \" {
			send \"test-password\r\"
			expect \"\rtype new password again: \" {
				send \"test-password\r\"
			}
		}

		send_user \"passed\"
		expect eof
	"

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
