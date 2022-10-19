run_deploy_revision() {
	echo

	model_name="test-deploy-revision"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# revision 23 is in channel 2.0/edge
	juju deploy juju-qa-test --revision 23 --channel 2.0/stable
	wait_for "juju-qa-test" "$(charm_rev "juju-qa-test" 23)"

	# check resource revision per channel specified.
	got=$(juju resources juju-qa-test --format json | jq -S '.resources[0] | .["revision"] == "1"')
	check_contains "${got}" "true"

	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"

	juju config juju-qa-test foo-file=true
	wait_for "resource line one: testing one." "$(workload_status juju-qa-test 0).message"

	# check resource revision again per channel specified.
	juju resources juju-qa-test --format json | jq -S '.resources[0] | .[ "revision"] == "1"'

	destroy_model "${model_name}"
}

run_deploy_revision_resource() {
	echo

	model_name="test-deploy-revision-resource"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# revision 23 is in channel 2.0/edge
	juju deploy juju-qa-test --revision 23 --channel 2.0/stable --resource foo-file=4
	wait_for "juju-qa-test" "$(charm_rev "juju-qa-test" 23)"

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

	# NOTE:
	# The following loop is specific to juju 3.0+ due to
	# async charm download and should NOT be removed in
	# a merge from 2.9.
	attempt=0
	while true; do
		# Ensure that refresh gets the revision from the channel
		# listed at deploy.
		# revision 15 is in channel latest/edge
		OUT=$(juju refresh juju-qa-test 2>&1 || true)
		if echo "${OUT}" | grep -E -q "Added"; then
			break
		fi
		attempt=$((attempt + 1))
		if [ $attempt -eq 10 ]; then
			# shellcheck disable=SC2046
			echo $(red "timeout: waiting for charm download to complete 50sec")
			exit 5
		fi
		sleep 5
	done

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
