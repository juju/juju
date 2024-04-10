# Simple test to check if branches work with application configuration
# and can be created, set, read back and commited.
run_branch() {
	# Echo out to ensure nice output to the test suite.
	echo

	# The following ensures that a bootstrap juju exists
	file="${TEST_DIR}/test-branches.log"
	ensure "branches" "${file}"

	juju branch | check 'Active branch is "master"'

	juju deploy juju-qa-dummy-source --base ubuntu@22.04 --config token=a

	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	juju add-branch test-branch
	juju branch | check 'Active branch is "test-branch"'

	juju config dummy-source token=b --branch test-branch
	check_config_command "juju config dummy-source token --branch test-branch" "b"
	check_config_command "juju config dummy-source token --branch master" "a"

	juju commit test-branch | check 'Active branch set to "master"'
	check_config_command "juju config dummy-source token" "b"

	# Clean up!
	destroy_model "branches"
}

test_branch() {
	if [ -n "$(skip 'test_branch')" ]; then
		echo "==> SKIP: Asked to skip branch tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_branch"
	)
}

# The check function reads stdout until a new-line is detected.
# This does not work for us, because config does not include a new line at the end
# of the output. Hence this function.
check_config_command() {
	local want got

	want=${2}

	got=$(eval "${1}")

	OUT=$(echo "${got}" | grep -E "${want}" || true)
	if [ -z "${OUT}" ]; then
		echo "" >&2
		# shellcheck disable=SC2059
		printf "$(red \"Expected\"): ${want}\n" >&2
		# shellcheck disable=SC2059
		printf "$(red \"Recieved\"): ${got}\n" >&2
		echo "" >&2
		exit 1
	fi
}
