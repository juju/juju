run_deploy_revision() {
	echo

	model_name="test-deploy-revision"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# revision 9 is in channel 2.0/edge
	juju deploy juju-qa-test --revision 9 --channel 2.0/stable
	wait_for "juju-qa-test" "$(charm_rev "juju-qa-test" 9)"

	# check resource revision per channel specified.
	juju resources juju-qa-test --format json | jq -S '.resources[0] | .[ "revision"] == "3"'

	destroy_model "${model_name}"
}

run_deploy_revision_resource() {
	echo

	model_name="test-deploy-revision-resource"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# revision 9 is in channel 2.0/edge
	juju deploy juju-qa-test --revision 9 --channel 2.0/stable --resource foo-file=4
	wait_for "juju-qa-test" "$(charm_rev "juju-qa-test" 9)"

	# check resource revision as specified in command.
	juju resources juju-qa-test --format json | jq -S '.resources[0] | .[ "revision"] == "4"'

	destroy_model "${model_name}"
}

run_deploy_revision_fail() {
	echo

	model_name="test-deploy-revision-fail"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	got=$(juju deploy juju-qa-test --revision 9 2>&1 || true)
	check_contains "${got}" 'ERROR invalid channel for "ch:juju-qa-test": channel cannot be empty'

	destroy_model "${model_name}"
}

run_deploy_revision_upgrade() {
	echo

	model_name="test-deploy-upgrade"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# revision 9 is in channel 2.0/edge
	juju deploy juju-qa-test --revision 9 --channel latest/edge
	wait_for "juju-qa-test" "$(charm_rev "juju-qa-test" 9)"

	# Ensure that upgrade-charm gets the revision from the channel
	# listed at deploy.
	# revision 15 is in channel latest/edge
	juju upgrade-charm juju-qa-test
	wait_for "juju-qa-test" "$(charm_rev "juju-qa-test" 15)"

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
		run "run_deploy_revision_resource"
		run "run_deploy_revision_fail"
		run "run_deploy_revision_upgrade"
	)
}
