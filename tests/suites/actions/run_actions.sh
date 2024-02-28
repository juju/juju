# Copyright 2024 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.

run_actions_params() {
	echo
	model_name="test-actions"
	log_file="${TEST_DIR}/${model_name}.log"

	ensure "juju-qa-action" "${log_file}"

	juju deploy juju-qa-action

	wait_for "juju-qa-action" "$(idle_condition "juju-qa-action")"

	# Run a valid action.
	juju run juju-qa-action/0 fortune length="long"
	# Check the the task succeeded.
	juju show-operation 1 --format=json | jq '.status' | check 'completed'
	# Check the action succeeded.
	juju show-task 2 --format=json | jq '.status' | check 'completed'

	# Run an action that will return failed status.
	juju run juju-qa-action/0 fortune length="long" fail="fail with this string"
	# Check the the task failed as expected.
	juju show-operation 3 --format=json | jq '.status' | check 'failed'
	# Check the action failed as expected.
	juju show-task 4 --format=json | jq '.status' | check 'failed'

	# Run an action with misspelled parameters.
	juju run juju-qa-action/0 fortune length="long" misspelledparam="ok"
	# Check the action was rejected.
	juju show-operation 5 --format=json | jq '.status' | check 'error'
	# Check the task did not run and has status error.
	juju show-task 6 --format=json | jq '.status' | check 'error'

	# Run an action that returns all the parameters passed to it.
	juju run juju-qa-action/0 list-my-params string="my string" array="[1,2]" bool=true
	# Check the the task succeeded.
	juju show-operation 7 --format=json | jq '.status' | check 'completed'
	# Check the params were returned in the results.
	juju show-task 8 --format=json | jq '.results | .["string"]' | check 'my string'
	juju show-task 8 --format=json | jq '.results | .["array"]' | check '[1, 2]'
	juju show-task 8 --format=json | jq '.results | .["bool"]' | check 'True'

	destroy_model "actions_test"
}

test_actions_params() {
	if [ "$(skip 'test_actions_params')" ]; then
		echo "==> TEST SKIPPED: actions params tests"
		return
	fi

	(
		set_verbosity

		run "run_actions_params"
	)
}
