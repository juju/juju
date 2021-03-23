run_relation_list_app() {
	echo

	model_name="test-relation-list-app"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# Deploy 2 departer instances
	juju deploy wordpress
	juju deploy mysql
	juju relate wordpress mysql
	wait_for "wordpress" "$(idle_condition "wordpress" 1 0)"
	wait_for "mysql" "$(idle_condition "mysql" 0 0)"

	# Figure out the right relation IDs to use for our hook tool invocations
	db_rel_id=$(juju run --unit mysql/0 "relation-ids db" | cut -d':' -f2)
	peer_rel_id=$(juju run --unit mysql/0 "relation-ids cluster" | cut -d':' -f2)

	# Remove wordpress unit; the wordpress-mysql relation is still established
	# but there are no units present in the wordpress side
	juju remove-unit wordpress/0
	sleep 5

	got=$(juju run --unit mysql/0 "relation-list --app -r ${db_rel_id}")
	if [ "${got}" != "wordpress" ]; then
		# shellcheck disable=SC2046
		echo $(red "expected running 'relation-list --app' on mysql unit for non-peer relation to return 'wordpress'; got ${got}")
		exit 1
	fi

	got=$(juju run --unit mysql/0 "relation-list --app -r ${peer_rel_id}")
	if [ "${got}" != "mysql" ]; then
		# shellcheck disable=SC2046
		echo $(red "expected running 'relation-list --app' on mysql unit for peer relation to return 'mysql'; got ${got}")
		exit 1
	fi

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
