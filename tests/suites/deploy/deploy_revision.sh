run_deploy_revision() {
	echo

	model_name="test-deploy-revision"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# revision 29 is in channel 2.0/edge
	juju deploy juju-qa-test --revision 29 --channel 2.0/edge
	wait_for "juju-qa-test" "$(charm_rev "juju-qa-test" 29)"

	# check resource revision per channel specified.
	got=$(juju resources juju-qa-test --format json | yq '.resources[0] | .["revision"] == "3"')
	check_contains "${got}" "true"

	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"

	juju config juju-qa-test foo-file=true
	wait_for "resource line one: testing one plus one." "$(workload_status juju-qa-test 0).message"

	# check resource revision again per channel specified.
	juju resources juju-qa-test --format json | yq '.resources[0] | .[ "revision"] == "3"'

	destroy_model "${model_name}"
}

# Verify when deploying a charm with a specific revision that the resource
# revision deployed is consistent with the charm revision that was released
# into the channel. This test could break if the charm revision is
# re-released into the channel with a different resource revision but this
# is not conventionally done.
run_deploy_revision_uses_consistent_resource() {
	echo

	model_name="test-deploy-revision-uses-consistent-resource"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# A charm publisher can view the previous charm revisions and resource
	# revisions released into a channel at https://charmhub.io/juju-qa-test/releases.
	juju deploy juju-qa-test --revision 31 --channel 2.0/candidate
	wait_for "juju-qa-test" "$(charm_rev "juju-qa-test" 31)"

	# Check resource revision per channel specified.
	# The latest resource revision released into channel 2.0/candidate is 3,
	# here we expect 2 as that was the resource revision released with charm revision 31.
	got=$(juju resources juju-qa-test --format json | yq '.resources[0] | .["revision"] == "2"')
	check_contains "${got}" "true"

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
	got=$(juju resources juju-qa-test --format json | yq '.resources[0] | .["revision"] == "4"')
	check_contains "${got}" "true"

	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"

	juju config juju-qa-test foo-file=true
	wait_for "resource line one: testing four." "$(workload_status juju-qa-test 0).message"

	# check resource revision again per channel specified.
	juju resources juju-qa-test --format json | yq '.resources[0] | .[ "revision"] == "4"'

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
	juju deploy juju-qa-test --revision 23 --channel 2.0/edge
	wait_for "juju-qa-test" "$(charm_rev "juju-qa-test" 23)"
	wait_for "juju-qa-test" "$(active_idle_condition "juju-qa-test")"

	# Once the application is ready, refresh is expected to immediately work.
	juju refresh juju-qa-test --channel latest/edge

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
		run "run_deploy_revision_uses_consistent_resource"
		run "run_deploy_revision_fail"
		run "run_deploy_revision_refresh"
		run "run_deploy_revision_resource"
	)
}
