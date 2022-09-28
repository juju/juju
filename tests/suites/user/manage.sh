# Granting and revoking read/write/admins rights for the users.
run_user_grant_revoke() {
	# Echo out to ensure nice output to the test suite.
	echo

	# The following ensures that a bootstrap juju exists.
	file="${TEST_DIR}/test-user-grant-revoke.log"
	ensure "user-grant-revoke" "${file}"

	echo "Check that current user is admin"
	juju whoami --format=json | jq -r '."user"' | check "admin"

	echo "Add user with read rights"
	juju add-user readuser
	juju grant readuser read "user-grant-revoke"

	echo "Add user with write rights"
	juju add-user writeuser
	juju grant writeuser write "user-grant-revoke"

	echo "Add user with admin rights"
	juju add-user adminuser
	juju grant adminuser admin "user-grant-revoke"

	echo "Check rights for added users"
	juju show-model "user-grant-revoke" --format=json | jq -r '."user-grant-revoke"."users"."readuser"."access"' | check "read"
	juju show-model "user-grant-revoke" --format=json | jq -r '."user-grant-revoke"."users"."writeuser"."access"' | check "write"
	juju show-model "user-grant-revoke" --format=json | jq -r '."user-grant-revoke"."users"."adminuser"."access"' | check "admin"

	echo "Revoke rights"
	juju revoke readuser read "user-grant-revoke"
	juju revoke writeuser write "user-grant-revoke"
	juju revoke adminuser admin "user-grant-revoke"

	echo "Check rights for added users after revoke"
	juju show-model "user-grant-revoke" --format=json | jq -r '."user-grant-revoke"."users"."readuser"."access"' | check null
	juju show-model "user-grant-revoke" --format=json | jq -r '."user-grant-revoke"."users"."writeuser"."access"' | check "read"
	juju show-model "user-grant-revoke" --format=json | jq -r '."user-grant-revoke"."users"."adminuser"."access"' | check "write"

	destroy_model "user-grant-revoke"
}

# Disabling and enabling users.
run_user_disable_enable() {
	# Echo out to ensure nice output to the test suite.
	echo

	# The following ensures that a bootstrap juju exists.
	file="${TEST_DIR}/test-user-disable-enable.log"
	ensure "user-disable-enable" "${file}"

	echo "Check that current user is admin"
	juju whoami --format=json | jq -r '."user"' | check "admin"

	echo "Add testuser"
	juju add-user testuser
	juju grant testuser read "user-disable-enable"

	echo "Disable testuser"
	juju disable-user testuser

	echo "Check testuser is disabled"
	juju show-user testuser --format=json | jq -r '."disabled"' | check true

	echo "Enable testuser"
	juju enable-user testuser

	echo "Check testuser is enabled"
	juju show-user testuser --format=json | jq -r '."disabled"' | check null

	destroy_model "user-disable-enable"
}

# Granting and revoking login/add-model/superuser rights for the controller access.
run_user_controller_access() {
	# Echo out to ensure nice output to the test suite.
	echo

	# The following ensures that a bootstrap juju exists.
	file="${TEST_DIR}/test-user-controller-access.log"
	ensure "user-controller-access" "${file}"

	echo "Check that current user is admin"
	juju whoami --format=json | jq -r '."user"' | check "admin"

	echo "Add user with login rights"
	juju add-user junioradmin

	echo "Add user with superuser rights"
	juju add-user senioradmin
	juju grant senioradmin superuser

	echo "Check rights for added users"
	juju users --format=json | jq -r '.[] | select(."user-name"=="junioradmin") | ."access"' | check "login"
	juju users --format=json | jq -r '.[] | select(."user-name"=="senioradmin") | ."access"' | check "superuser"

	echo "Revoke rights"
	juju revoke junioradmin login
	juju revoke senioradmin superuser

	echo "Check rights for added users after revoke"
	juju users --format=json | jq -r '.[] | select(."user-name"=="junioradmin") | ."access"' | check ""
	juju users --format=json | jq -r '.[] | select(."user-name"=="senioradmin") | ."access"' | check "login"

	destroy_model "user-controller-access"
}

test_user_manage() {
	if [ -n "$(skip 'test_user_manage')" ]; then
		echo "==> SKIP: Asked to skip user manage tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_user_grant_revoke"
		run "run_user_disable_enable"
		run "run_user_controller_access"
	)
}
