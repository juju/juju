# Copyright 2024 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.

run_relation_model_get() {
	echo

	model_name="test-relation-model-get"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju deploy juju-qa-dummy-sink
	juju deploy juju-qa-dummy-source --config token=becomegreen

	echo "Establish relation"
	juju integrate dummy-sink dummy-source

	wait_for "dummy-sink" "$(idle_condition "dummy-sink" 0)"
	wait_for "dummy-source" "$(idle_condition "dummy-source" 0)"

	echo "Figure out the right relation IDs to use for hook command invocations"
	sink_rel_id=$(juju exec --unit dummy-source/0 "relation-ids sink" | cut -d':' -f2)

	echo "Check relation-model-get hook"
	model_uuid=$(juju show-model --format json | jq -r '.["test-relation-model-get"]["model-uuid"]')
	juju exec --unit dummy-source/0 "relation-model-get -r ${sink_rel_id}" | check "${model_uuid}"

	echo "Setting up cross model relation"
	juju offer dummy-source:sink

	another_model_name="test-relation-model-get-another"
	juju add-model "${another_model_name}"

	juju deploy juju-qa-dummy-sink
	juju integrate dummy-sink "${model_name}.dummy-source"

	wait_for "dummy-sink" "$(idle_condition "dummy-sink" 0)"

	echo "Figure out the right relation IDs to use for hook command invocations"
	sink_rel_id=$(juju exec --unit dummy-sink/0 "relation-ids source" | cut -d':' -f2)

	echo "Check relation-model-get hook"
	juju exec --unit dummy-sink/0 "relation-model-get -r ${sink_rel_id}" | check "${model_uuid}"

	destroy_model "${model_name}"
	destroy_model "${another_model_name}"
}

test_relation_model_get() {
	if [ "$(skip 'test_relation_model_get')" ]; then
		echo "==> TEST SKIPPED: relation model get unit tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_relation_model_get"
	)
}
