# Checks juju that juju shows no branches output.
run_indicate_active_branch_no_active() {
	echo

	file="${TEST_DIR}/indicate-active-branch-no-active.log"

	ensure "indicate-active-branch-no-active" "${file}"

	juju deploy cs:~jameinel/ubuntu-lite-7

	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"

	check_not_contains "$(juju status)" "Branch"

	if [ "$(juju status --format=json | jq '.branches')" != null ]; then
		echo "The status shows branches even though we do not use them yet"
		exit 1
	fi

	destroy_model "indicate-active-branch-no-active"
}

# Checks juju that juju shows branches output as we are using it
run_indicate_active_branch_active() {
	echo

	file="${TEST_DIR}/indicate-active-branch-active.log"

	ensure "indicate-active-branch-active" "${file}"

	juju deploy cs:~jameinel/ubuntu-lite-7

	juju add-branch bla
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"

	check_contains "$(juju status)" "bla\*"

	if [ "$(juju status --format=json | jq '.branches.bla.active')" != true ]; then
		echo "The status does not show active branch"
		exit 1
	fi

	juju add-branch testtest
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"

	# juju status can be slow, we might need to wait for testtest to appear
	wait_for "active" ".branches.testtest"

	check_contains "$(juju status)" "testtest\*"
	check_not_contains "$(juju status)" "bla\*"

	STATUS_UNACTIVE=$(juju status --format=json | jq '.branches.bla.active')
	STATUS_ACTIVE=$(juju status --format=json | jq '.branches.testtest.active')

	if [ "${STATUS_UNACTIVE}" != null ]; then
		echo "The status shows active branch"
		exit 1
	fi

	if [ "${STATUS_ACTIVE}" != true ]; then
		echo "The status shows active branch"
		exit 1
	fi

	destroy_model "indicate-active-branch-active"
}

test_active_branch_output() {
	if [ "$(skip 'test_active_branch_output')" ]; then
		echo "==> TEST SKIPPED: test_active_branch_outputtests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_indicate_active_branch_no_active"
		run "run_indicate_active_branch_active"
	)
}
