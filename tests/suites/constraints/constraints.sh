run_constraints_aws() {
  # Echo out to ensure nice output to the test suite.
  echo

  file="${TEST_DIR}/constraints-aws.txt"

  ensure "constraints-aws" "${file}"

	echo "Deploy 3 machines with different constraints"
  juju add-machine --constraints "root-disk=16G"
  juju add-machine --constraints "cores=4 root-disk=16G"
  juju add-machine --constraints "instance-type=t2.nano"

  wait_for_machine_agent_status "0" "started"
  wait_for_machine_agent_status "1" "started"
  wait_for_machine_agent_status "2" "started"

	echo "Ensure machine 0 has 16G root disk"
  machine0_hardware=$(juju machines --format json | jq -r '.["machines"]["0"]["hardware"]')
  machine0_rootdisk=$(echo $machine0_hardware | awk '{for(i=1;i<=NF;i++){if($i ~ /root-disk/){print $i}}}')
  check_ge "${machine0_rootdisk}" "root-disk=16384M"

	echo "Ensure machine 1 has 4 cores and 16G root disk"
  machine1_hardware=$(juju machines --format json | jq -r '.["machines"]["1"]["hardware"]')
  machine1_cores=$(echo $machine1_hardware | awk '{for(i=1;i<=NF;i++){if($i ~ /cores/){print $i}}}')
  machine1_rootdisk=$(echo $machine1_hardware | awk '{for(i=1;i<=NF;i++){if($i ~ /root-disk/){print $i}}}')
  check_ge "${machine1_cores}" "cores=4"
  check_ge "${machine1_rootdisk}" "root-disk=16384M"

	echo "Ensure machine 2 has t2.nano instance type"
  machine2_constraints=$(juju machines --format json | jq -r '.["machines"]["2 "]["constraints"]')
  check_contains "${machine2_constraints}" "instance-type=t2.nano"

  destroy_model "constraints-aws"
}

test_constraints_aws() {
  if [ "$(skip 'test_constraints_aws')" ]; then
  		echo "==> TEST SKIPPED: constraints aws"
  		return
  fi

  (
  		set_verbosity

  		cd .. || exit

  		run "run_constraints_aws"
  	)
}
