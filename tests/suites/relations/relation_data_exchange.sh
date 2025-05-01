# Ensure that related applications can exchange data via databag correctly.
run_relation_data_exchange() {
	echo

	model_name="test-relation-data-exchange"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	echo "Deploy 2 dummy-sink instances and one dummy-source instance"

	juju deploy juju-qa-dummy-sink -n 2
	juju deploy juju-qa-dummy-source --config token=becomegreen

	echo "Establish relation"
	juju relate dummy-sink dummy-source

	wait_for "dummy-sink" "$(idle_condition "dummy-sink" 0 0)"
	wait_for "dummy-sink" "$(idle_condition "dummy-sink" 0 1)"
	wait_for "dummy-source" "$(idle_condition "dummy-source" 1 0)"

	echo "Get the leader unit name"
	non_leader_dummy_sink_unit=$(juju status dummy-sink --format json | jq -r '.applications."dummy-sink".units | to_entries[] | select(.value.leader!=true) | .key')
	dummy_sink_relation_id=$(juju exec --unit "dummy-sink/leader" 'relation-ids source')
	dummy_source_relation_id=$(juju exec --unit "dummy-source/leader" 'relation-ids sink')
	# stop there
	echo "Block until the relation is joined; otherwise, the relation-set commands below will fail"
	attempt=0
	while true; do
		got=$(juju exec --unit "dummy-sink/leader" "relation-get --app -r ${dummy_sink_relation_id} origin dummy-sink" || echo 'NOT FOUND')
		if [ "${got}" != "NOT FOUND" ]; then
			break
		fi
		attempt=$((attempt + 1))
		if [ $attempt -eq 30 ]; then
			# shellcheck disable=SC2046
			echo $(red "timeout: dummy-sink has not yet joined sink relation after 30sec")
			exit 1
		fi
		sleep 1
	done
	attempt=0
	while true; do
		got=$(juju exec --unit 'dummy-source/0' "relation-get --app -r ${dummy_sink_relation_id} origin dummy-source" || echo 'NOT FOUND')
		if [ "${got}" != "NOT FOUND" ]; then
			break
		fi
		attempt=$((attempt + 1))
		if [ $attempt -eq 30 ]; then
			# shellcheck disable=SC2046
			echo $(red "timeout: dummy-source has not yet joined sink relation after 30sec")
			exit 1
		fi
		sleep 1
	done

	echo "Exchange relation data"
	juju exec --unit 'dummy-source/0' "relation-set --app -r ${dummy_source_relation_id} origin=dummy-source"
	# As the leader units, set some *application* data for both sides of a non-peer relation
	juju exec --unit "dummy-sink/leader" "relation-set --app -r ${dummy_sink_relation_id} origin=dummy-sink"
	juju exec --unit 'dummy-source/0' "relation-set --app -r ${dummy_source_relation_id} origin=dummy-source"

	echo "Check 1: ensure that leaders can read the application databag for their own application"
	juju exec --unit "dummy-sink/leader" "relation-get --app -r ${dummy_sink_relation_id} origin dummy-sink" | check "dummy-sink"
	juju exec --unit 'dummy-source/0' "relation-get --app -r ${dummy_source_relation_id} origin dummy-source" | check "dummy-source"

	echo "Check 2: ensure that non-leader units are not allowed to read their own application databag for non-peer relations"
	juju exec --unit "${non_leader_dummy_sink_unit}" "relation-get --app -r ${dummy_sink_relation_id} origin dummy-sink" 2>&1 || echo "PERMISSION DENIED" | check "PERMISSION DENIED"

	destroy_model "${model_name}"
}

test_relation_data_exchange() {
	if [ "$(skip 'test_relation_data_exchange')" ]; then
		echo "==> TEST SKIPPED: relation data exchange tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_relation_data_exchange"
	)
}
