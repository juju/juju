# Ensure that relation-list hook works correctly.
run_relation_list_app() {
	echo

	model_name="test-relation-list-app"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	echo "Deploy 2 departer instances"
	juju deploy mysql --channel=8.0/stable --force --series jammy
	juju deploy wordpress --force --series bionic

	juju relate wordpress mysql
	wait_for "wordpress" "$(idle_condition "wordpress" 1 0)"
	wait_for "mysql" "$(idle_condition "mysql" 0 0)"

	echo "Figure out the right relation IDs to use for our hook tool invocations"
	db_rel_id=$(juju exec --unit mysql/0 "relation-ids mysql" | cut -d':' -f2)
	peer_rel_id=$(juju exec --unit mysql/0 "relation-ids database-peers" | cut -d':' -f2)

	echo "Remove wordpress unit"
	# the wordpress-mysql relation is still established but there are no units present in the wordpress side
	juju remove-unit wordpress/0
	wait_for null '.applications."wordpress".units."wordpress/0"'

	echo "Check relation-list hook"
	juju exec --unit mysql/0 "relation-list --app -r ${db_rel_id}" | check "wordpress"
	juju exec --unit mysql/0 "relation-list --app -r ${peer_rel_id}" | check "mysql"

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
