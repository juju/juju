run_deploy_revision() {
	echo

	model_name="test-deploy-revision"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# revision 29 is in channel 2.0/edge
	juju deploy juju-qa-test --revision 29 --channel 2.0/edge
	wait_for "juju-qa-test" "$(charm_rev "juju-qa-test" 29)"

	# check resource revision per channel specified.
	got=$(juju resources juju-qa-test --format json | jq -S '.resources[0] | .["revision"] == "3"')
	check_contains "${got}" "true"

	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"

	juju config juju-qa-test foo-file=true
	wait_for "resource line one: testing one plus one." "$(workload_status juju-qa-test 0).message"

	# check resource revision again per channel specified.
	juju resources juju-qa-test --format json | jq -S '.resources[0] | .[ "revision"] == "3"'

	destroy_model "${model_name}"
}

run_deploy_revision_resource() {
	echo

	model_name="test-deploy-revision-resource"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# revision 29 is in channel 2.0/edge
	juju deploy juju-qa-test --revision 29 --channel 2.0/edge --resource foo-file=4
	wait_for "juju-qa-test" "$(charm_rev "juju-qa-test" 29)"

	# check resource revision as specified in command.
	got=$(juju resources juju-qa-test --format json | jq -S '.resources[0] | .["revision"] == "4"')
	check_contains "${got}" "true"

	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"

	juju config juju-qa-test foo-file=true
	wait_for "resource line one: testing four." "$(workload_status juju-qa-test 0).message"

	# check resource revision again per channel specified.
	juju resources juju-qa-test --format json | jq -S '.resources[0] | .[ "revision"] == "4"'

	destroy_model "${model_name}"
}

run_deploy_revision_fail() {
	echo

	model_name="test-deploy-revision-fail"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	got=$(juju deploy juju-qa-test --revision 9 2>&1 || true)
	# bad request should be caught by client
	check_contains "${got}" 'revision requires a channel for future upgrades'

	destroy_model "${model_name}"
}

run_deploy_revision_refresh() {
	echo

	model_name="test-deploy-refresh"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# revision 23 is in channel 2.0/edge
	juju deploy juju-qa-test --revision 23 --channel latest/edge
	wait_for "juju-qa-test" "$(charm_rev "juju-qa-test" 23)"
	wait_for "juju-qa-test" "$(active_idle_condition "juju-qa-test")"

	# Once the application is ready, refresh is expected to immediately work.
	juju refresh juju-qa-test

	# revision 21 is in channel latest/edge
	wait_for "juju-qa-test" "$(charm_rev "juju-qa-test" 21)"
	wait_for "juju-qa-test" "$(charm_channel "juju-qa-test" "latest/edge")"

	destroy_model "${model_name}"
}

test_deploy_revision() {
	if [ "$(skip 'test_deploy_revision')" ]; then
		echo "==> TEST SKIPPED: deploy revision"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_deploy_revision"
		run "run_deploy_revision_fail"
		run "run_deploy_revision_refresh"
		run "run_deploy_revision_resource"
	)
}
