# Copyright 2024 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.

# Ensure that relation-list hook works correctly.
run_relation_list_app() {
	echo

	model_name="test-relation-list-app"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	echo "Deploy 2 departer instances"

	juju deploy juju-qa-dummy-sink
	juju deploy juju-qa-dummy-source --config token=becomegreen

	echo "Establish relation"
	juju relate dummy-sink dummy-source

	wait_for "dummy-sink" "$(idle_condition "dummy-sink" 0 0)"
	wait_for "dummy-source" "$(idle_condition "dummy-source" 1 0)"

	echo "Figure out the right relation IDs to use for our hook tool invocations"
	sink_rel_id=$(juju exec --unit dummy-source/0 "relation-ids sink" | cut -d':' -f2)

	echo "Remove dummy-sink unit"
	# the dummy-sink-source relation is still established but there are no units present in the dummy-sink side
	juju remove-unit dummy-sink/0
	wait_for null '.applications."dummy-sink".units."dummy-sink/0"'

	echo "Check relation-list hook"
	juju exec --unit dummy-source/0 "relation-list --app -r ${sink_rel_id}" | check "dummy-sink"

	destroy_model "${model_name}"
}

test_relation_list_app() {
	if [ "$(skip 'test_relation_list_app')" ]; then
		echo "==> TEST SKIPPED: relation list app unit tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_relation_list_app"
	)
}
