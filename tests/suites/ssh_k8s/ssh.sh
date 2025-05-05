run_ssh() {
	echo

	model_name='test-ssh'
	juju --show-log add-model "$model_name"
	model_uuid=$(juju show-model $model_name --format json | jq -r ".[\"${model_name}\"][\"model-uuid\"]")

	app_name="hello-kubecon"
	juju deploy $app_name
	wait_for $app_name "$(active_idle_condition $app_name 0 0)"

	controller_name=$(juju controllers --format json | jq -r '."current-controller"')
	controller_address=$(juju show-controller --format json | jq -r ".[\"${controller_name}\"][\"details\"][\"api-endpoints\"][0]" | cut -d ':' -f 1)

	container_name="charm"
	container_hostname=$container_name.0.$app_name.$model_uuid.juju.local

	check_ssh_using_openssh "$controller_address" "$container_hostname"

	# TODO: Add tests for the Juju CLI below

	destroy_model "${model_name}"
}

check_ssh_using_openssh() {
	controller_address=${1}
	virtual_hostname=${2}
	test_file="test-ssh.txt"

	# Check that we can write a file on the remote host then read it back.
	ssh_no_hostkey_check -J admin@"$controller_address":17022 ubuntu@"$virtual_hostname" "echo hello > $test_file"
	output=$(ssh_no_hostkey_check -J admin@"$controller_address":17022 ubuntu@"$virtual_hostname" "cat $test_file")
	check_contains "$output" hello
}

ssh_no_hostkey_check() {
	ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null "$@"
}

test_ssh() {
	if [ "$(skip 'test_ssh')" ]; then
		echo "==> TEST SKIPPED: test_ssh"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_ssh"
	)
}
