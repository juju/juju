# Deploy some machines with spaces constraints and then
# check that they have been deployed as expected
run_machines_in_spaces() {
	echo

	model_name='machines-in-spaces'
	juju --show-log add-model "$model_name"

	echo "Setup spaces"
	juju reload-spaces
	juju add-space space1 10.104.0.0/20

	juju add-machine --constraints spaces=alpha -n2
	juju add-machine --constraints spaces=space1

	wait_for_machine_agent_status "0" "started"
	wait_for_machine_agent_status "1" "started"
	wait_for_machine_agent_status "2" "started"

	echo "Verify machines are assigned to correct spaces"
	alpha_cidrs="$(juju spaces --format json | jq -r '.spaces[] | select(.name == "alpha").subnets | to_entries[] | select(.value["provider-id"] | contains("INFAN") | not) | .key')"
	assert_machine_ip_is_in_cidrs "0" "${alpha_cidrs}"
	machine_1_space_ip=$(assert_machine_ip_is_in_cidrs "1" "${alpha_cidrs}")
	machine_2_space_ip=$(assert_machine_ip_is_in_cidrs "2" "10.104.0.0/20")

	echo "Verify machines can ping each other within and across spaces"
	juju ssh 0 "ping -c4 ${machine_1_space_ip}"
	juju ssh 0 "ping -c4 ${machine_2_space_ip}"

	echo "Attempt assigning a container to a different space to it's host machine and assert this fails"
	juju add-machine lxd:0 --constraints spaces=space1
	wait_for "provisioning error" '.machines["0"].containers["0/lxd/0"]["machine-status"].current'

	# A container lying around in error state will cause destroy_model to return non-zero
	echo "Destroy container"
	juju remove-machine "0/lxd/0"
	wait_for "false" '.machines["0"] | has("containers")'

	destroy_model ${model_name}
}

test_machines_in_spaces() {
	if [ "$(skip 'test_machines_in_spaces')" ]; then
		echo "==> TEST SKIPPED: assert machines added to spaces"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_machines_in_spaces" "$@"
	)
}
