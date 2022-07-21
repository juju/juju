run_constraints_lxd() {
	# Echo out to ensure nice output to the test suite.
	echo

	file="${TEST_DIR}/constraints-lxd.txt"

	ensure "constraints-lxd" "${file}"

	echo "Deploy 2 machines with different constraints"
	juju add-machine --constraints "cores=2"
	juju add-machine --constraints "cores=2 mem=2G"

	wait_for_machine_agent_status "0" "started"
	wait_for_machine_agent_status "1" "started"

	echo "Ensure machine 0 has 2 cores"
	machine0_hardware=$(juju machines --format json | jq -r '.["machines"]["0"]["hardware"]')
	check_contains "${machine0_hardware}" "cores=2"

	echo "Ensure machine 1 has 2 cores and 2G memory"
	machine1_hardware=$(juju machines --format json | jq -r '.["machines"]["1"]["hardware"]')
	check_contains "${machine1_hardware}" "cores=2"
	check_contains "${machine1_hardware}" "mem=2048M"

	destroy_model "constraints-lxd"
}
