run_state_delete_get_set() {
	echo

	model_name="test-state-delete-get-set"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju deploy ubuntu-lite
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"

	juju exec --unit ubuntu-lite/0 'state-get | grep -q "{}"'
	juju exec --unit ubuntu-lite/0 'state-set one=two'
	juju exec --unit ubuntu-lite/0 'state-get | grep -q "one: two"'
	juju exec --unit ubuntu-lite/0 'state-set three=four'
	juju exec --unit ubuntu-lite/0 'state-get three | grep -q "four"'
	juju exec --unit ubuntu-lite/0 'state-delete one'
	juju exec --unit ubuntu-lite/0 'state-get | grep -q "three: four"'
	juju exec --unit ubuntu-lite/0 'state-get one --strict | grep -q "ERROR \"one\" not found" || true'
	juju exec --unit ubuntu-lite/0 'state-get one'

	destroy_model "${model_name}"
}

# This is probably a relic of old 3.6 where unit state and uniter state where sharing a doc in mongo.
# In 4.0, both use the same state method to be updated but they update different table.
# This ensures that setting the status of an application (`status-set`) doesn't erase or discard
# a predefined state (`state-set`)
run_state_set_clash_uniter_state() {
	echo

	model_name="test-state-set-clash-uniter-state"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju deploy ubuntu-lite app
	wait_for "app" "$(idle_condition "app")"

	juju exec --unit app/0 'state-get | grep -q "{}"'
	juju exec --unit app/0 'state-set one=two'
	juju exec --unit app/0 'state-get | grep -q "one: two"'

	# update status
	juju exec --unit app/0 -- status-set active "update-status ran: $(date +"%H:%M")"

	# verify charm set values
	juju exec --unit app/0 'state-get | grep -q "one: two"'

	destroy_model "${model_name}"
}

test_state_hook_tools() {
	if [ "$(skip 'test_state_hook_tools')" ]; then
		echo "==> TEST SKIPPED: state hook tools"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_state_delete_get_set"
		run "run_state_set_clash_uniter_state"
	)
}
