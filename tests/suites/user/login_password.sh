# expect_command the ability to work with interactive commands with expect tool.
# The output is "OK", if tests passes successfully.
# The expect_script argument is a expect script body.
# The default timeout is 10 seconds.
#
# ```
# wait_for <command> <expect_script> [<timeout>]
# ```
expect_command() {
	local command expect_script

	command=${1}
	filename=$(echo "${command}" | tr ' ' '-')
	expect_script=${2}
	timeout=${3:-10} # default timeout: 10s

	cat >"${TEST_DIR}/${filename}.exp" <<EOF
#!/usr/bin/expect
proc abort { } { send_user \"\rTimeout Error!\" ; exit 2 }
expect_before timeout abort

set timeout ${timeout}
spawn ${command}
match_max 100000

${expect_script}

send_user \"OK\"
expect eof
EOF
	expect "${TEST_DIR}/${filename}.exp"
}

# run_user_change_password changes the admin password, logout and tries to login with new password
run_user_change_password() {
	# Echo out to ensure nice output to the test suite.
	echo

	# The following ensures that a bootstrap juju exists.
	file="${TEST_DIR}/test-user-change-password.log"
	ensure "user-change-password" "${file}"

	echo "Change admin password"
	expect_command "juju change-user-password" "
expect \"new password: \" {
	send \"test-password\r\"
	expect \"type new password again: \" {
		send \"test-password\r\"
	}
}" | check "OK"

	echo "Logout from controller"
	juju logout

	echo "Login as admin"
	expect_command "juju login" "
expect \"Enter username: \" {
	send \"admin\r\"
	expect \"please enter password for admin*\" {
		send \"test-password\r\"
	}
}" | check "OK"

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
