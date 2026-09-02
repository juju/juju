test_ssh_machine() {
	if [ "$(skip 'test_ssh_machine')" ]; then
		echo "==> TEST SKIPPED: machine SSH and SCP tests"
		return
	fi

	local model_name local_file remote_file copied_file output
	model_name="test-ssh-machine"
	add_model "${model_name}"

	juju deploy juju-qa-test ssh-test --base ubuntu@22.04
	wait_for "ssh-test" "$(idle_condition "ssh-test")"

	# Verify command execution on the machine hosting the unit.
	# shellcheck disable=SC2016
	output=$(juju ssh ssh-test/0 -- printf 'ssh-machine:%s $(hostname)')
	check_contains "${output}" "ssh-machine:juju-"

	# Verify command execution using the machine target syntax.
	output=$(juju ssh 0 -- printf machine-target)
	check_contains "${output}" "machine-target"

	local_file="${TEST_DIR}/machine-local.txt"
	remote_file="/tmp/juju-ssh-suite.txt"
	copied_file="${TEST_DIR}/machine-copied.txt"
	echo "machine-scp-test" >"${local_file}"

	# Copy to the machine and read the result remotely.
	juju scp "${local_file}" "0:${remote_file}"
	output=$(juju ssh 0 -- cat "${remote_file}")
	check_contains "${output}" "machine-scp-test"

	# Copy back from the machine and verify the local file contents.
	juju scp "0:${remote_file}" "${copied_file}"
	check_contains "$(cat "${copied_file}")" "machine-scp-test"

	destroy_model "${model_name}"
}

test_ssh_k8s() {
	if [ "$(skip 'test_ssh_k8s')" ]; then
		echo "==> TEST SKIPPED: Kubernetes SSH and SCP tests"
		return
	fi

	local model_name local_file remote_file copied_file output
	model_name="test-ssh-k8s"
	add_model "${model_name}"

	juju deploy snappass-test ssh-test
	wait_for "ssh-test" "$(idle_condition "ssh-test")"

	# The default target is the charm container. The explicit container target
	# exercises the same route used by sidecar workloads.
	output=$(juju ssh ssh-test/0 -- printf operator-container)
	check_contains "${output}" "operator-container"

	output=$(juju ssh --container redis ssh-test/0 -- printf workload-container)
	check_contains "${output}" "workload-container"

	local_file="${TEST_DIR}/k8s-local.txt"
	remote_file="/tmp/juju-ssh-suite.txt"
	copied_file="${TEST_DIR}/k8s-copied.txt"
	echo "k8s-scp-test" >"${local_file}"

	# Copy to and from the sidecar container.
	juju scp --container redis "${local_file}" "ssh-test/0:${remote_file}"
	output=$(juju ssh --container redis ssh-test/0 -- cat "${remote_file}")
	check_contains "${output}" "k8s-scp-test"

	juju scp --container redis "ssh-test/0:${remote_file}" "${copied_file}"
	check_contains "$(cat "${copied_file}")" "k8s-scp-test"

	destroy_model "${model_name}"
}
