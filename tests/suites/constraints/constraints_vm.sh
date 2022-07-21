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
	machine0_hardware=$(juju machines --format json | jq -r '.["machines"]["0"]["hardware"]')
	machine0_rootdisk=$(echo "$machine0_hardware" | awk '{for(i=1;i<=NF;i++){if($i ~ /root-disk/){print $i}}}')
	check_ge "${machine0_rootdisk}" "root-disk=16384M"

	echo "Ensure machine 1 has 4 cores and 16G root disk"
	machine1_hardware=$(juju machines --format json | jq -r '.["machines"]["1"]["hardware"]')
	machine1_cores=$(echo "$machine1_hardware" | awk '{for(i=1;i<=NF;i++){if($i ~ /cores/){print $i}}}')
	machine1_rootdisk=$(echo "$machine1_hardware" | awk '{for(i=1;i<=NF;i++){if($i ~ /root-disk/){print $i}}}')
	check_ge "${machine1_cores}" "cores=4"
	check_ge "${machine1_rootdisk}" "root-disk=16384M"

	destroy_model "constraints-vm"
}
