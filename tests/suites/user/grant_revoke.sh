# Granting and revoke read/write rights for a user.
run_user_grant_revoke() {
	# Echo out to ensure nice output to the test suite.
	echo

	# The following ensures that a bootstrap juju exists.
	file="${TEST_DIR}/test-user-grant-revoke.log"
	ensure "user-grant-revoke" "${file}"

	echo "Check that current user is admin"
	juju whoami --format=json | jq -r ".\"user\"" | check "admin"

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
	juju show-model "user-grant-revoke" --format=json | jq -r ".\"user-grant-revoke\".\"users\".\"readuser\".\"access\"" | check "read"
	juju show-model "user-grant-revoke" --format=json | jq -r ".\"user-grant-revoke\".\"users\".\"writeuser\".\"access\"" | check "write"
	juju show-model "user-grant-revoke" --format=json | jq -r ".\"user-grant-revoke\".\"users\".\"adminuser\".\"access\"" | check "admin"

	echo "Revoke rights"
	juju revoke readuser read "user-grant-revoke"
	juju revoke writeuser write "user-grant-revoke"
	juju revoke adminuser admin "user-grant-revoke"

	echo "Check rights for added users after revoke"
	juju show-model "user-grant-revoke" --format=json | jq -r ".\"user-grant-revoke\".\"users\".\"readuser\".\"access\"" | check null
	juju show-model "user-grant-revoke" --format=json | jq -r ".\"user-grant-revoke\".\"users\".\"writeuser\".\"access\"" | check "read"
	juju show-model "user-grant-revoke" --format=json | jq -r ".\"user-grant-revoke\".\"users\".\"adminuser\".\"access\"" | check "write"

	destroy_model "user-grant-revoke"
}

test_user_grant_revoke() {
	if [ -n "$(skip 'test_user_grant_revoke')" ]; then
		echo "==> SKIP: Asked to skip user grant revoke tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_user_grant_revoke"
	)
}