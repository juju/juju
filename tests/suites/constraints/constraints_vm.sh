run_constraints_vm() {
	# Echo out to ensure nice output to the test suite.
	echo

	file="${TEST_DIR}/constraints-vm.txt"

	ensure "constraints-vm" "${file}"

	echo "Deploy 2 machines with different constraints"
	juju add-machine --constraints "root-disk=16G"
	juju add-machine --constraints "cores=4 root-disk=16G"

	wait_for_machine_agent_status "0" "started"
	wait_for_machine_agent_status "1" "started"

	echo "Ensure machine 0 has 16G root disk"
	machine0_hardware=$(juju machines --format json | yq -r '.["machines"]["0"]["hardware"]')
	machine0_rootdisk=$(echo "$machine0_hardware" | awk '{for(i=1;i<=NF;i++){if($i ~ /root-disk/){print $i}}}')
	check_ge "${machine0_rootdisk}" "root-disk=16384M"

	echo "Ensure machine 1 has 4 cores and 16G root disk"
	machine1_hardware=$(juju machines --format json | yq -r '.["machines"]["1"]["hardware"]')
	machine1_cores=$(echo "$machine1_hardware" | awk '{for(i=1;i<=NF;i++){if($i ~ /cores/){print $i}}}')
	machine1_rootdisk=$(echo "$machine1_hardware" | awk '{for(i=1;i<=NF;i++){if($i ~ /root-disk/){print $i}}}')
	check_ge "${machine1_cores}" "cores=4"
	check_ge "${machine1_rootdisk}" "root-disk=16384M"

	# Model constraints must propagate to machines added afterwards.
	# Regression test for https://github.com/juju/juju/issues/22735
	echo "Deploy a machine with model constraints applied"
	juju set-model-constraints "cores=4"
	juju add-machine

	wait_for_machine_agent_status "2" "started"

	echo "Ensure machine 2 honours the model constraint cores=4"
	machine2_hardware=$(juju machines --format json | yq -r '.["machines"]["2"]["hardware"]')
	machine2_cores=$(echo "$machine2_hardware" | awk '{for(i=1;i<=NF;i++){if($i ~ /cores/){print $i}}}')
	check_ge "${machine2_cores}" "cores=4"

	destroy_model "constraints-vm"
}
