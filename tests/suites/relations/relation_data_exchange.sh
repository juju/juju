# Ensure that related applications can exchange data via databag correctly.
run_relation_data_exchange() {
	echo

	model_name="test-relation-data-exchange"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	echo "Deploy 2 wordpress instances and one mysql instance"
	juju deploy wordpress -n 2 --force --series bionic
	# mysql charm does not have stable channel, so we use edge channel
	juju deploy mysql --channel=edge --force --series focal
	wait_for "wordpress" "$(idle_condition "wordpress" 1 0)"
	wait_for "wordpress" "$(idle_condition "wordpress" 1 1)"
	wait_for "mysql" "$(idle_condition "mysql")"

	echo "Establish relation"
	juju relate wordpress mysql

	echo "Get the leader unit name"
	leader_wordpress_unit=$(juju status wordpress --format json | jq -r ".applications.wordpress.units | to_entries[] | select(.value.leader==true) | .key")
	non_leader_wordpress_unit=$(juju status wordpress --format json | jq -r ".applications.wordpress.units | to_entries[] | select(.value.leader!=true) | .key")

	echo "Block until the relation is joined; otherwise, the relation-set commands below will fail"
	attempt=0
	while true; do
		got=$(juju exec --unit "${leader_wordpress_unit}" 'relation-get --app -r db:2 origin wordpress' || echo 'NOT FOUND')
		if [ "${got}" != "NOT FOUND" ]; then
			break
		fi
		attempt=$((attempt + 1))
		if [ $attempt -eq 30 ]; then
			# shellcheck disable=SC2046
			echo $(red "timeout: wordpress has not yet joined db relation after 30sec")
			exit 1
		fi
		sleep 1
	done
	attempt=0
	while true; do
		got=$(juju exec --unit 'mysql/0' 'relation-get --app -r db:2 origin mysql' || echo 'NOT FOUND')
		if [ "${got}" != "NOT FOUND" ]; then
			break
		fi
		attempt=$((attempt + 1))
		if [ $attempt -eq 30 ]; then
			# shellcheck disable=SC2046
			echo $(red "timeout: mysql has not yet joined db relation after 30sec")
			exit 1
		fi
		sleep 1
	done

	echo "Exchange relation data"
	juju exec --unit 'mysql/0' 'relation-set --app -r db:2 origin=mysql'
	# As the leader units, set some *application* data for both sides of a non-peer relation
	juju exec --unit "${leader_wordpress_unit}" 'relation-set --app -r db:2 origin=wordpress'
	juju exec --unit 'mysql/0' 'relation-set --app -r db:2 origin=mysql'
	# As the leader wordpress unit, also set *application* data for a peer relation
	juju exec --unit "${leader_wordpress_unit}" 'relation-set --app -r loadbalancer:0 visible=to-peers'

	echo "Check 1: ensure that leaders can read the application databag for their own application"
	juju exec --unit "${leader_wordpress_unit}" 'relation-get --app -r db:2 origin wordpress' | check "wordpress"
	juju exec --unit 'mysql/0' 'relation-get --app -r db:2 origin mysql' | check "mysql"

	echo "Check 2: ensure that any unit can read its own application databag for *peer* relations"
	juju exec --unit "${leader_wordpress_unit}" 'relation-get --app -r loadbalancer:0 visible wordpress' | check "to-peers"
	juju exec --unit "${non_leader_wordpress_unit}" 'relation-get --app -r loadbalancer:0 visible wordpress' | check "to-peers"

	echo "Check 3: ensure that non-leader units are not allowed to read their own application databag for non-peer relations"
	juju exec --unit "${non_leader_wordpress_unit}" 'relation-get --app -r db:2 origin wordpress' 2>&1 || echo "PERMISSION DENIED" | check "PERMISSION DENIED"

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
