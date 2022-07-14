run_constraints_aws() {
  # Echo out to ensure nice output to the test suite.
  echo

  file="${TEST_DIR}/constraints-aws.txt"

  ensure "constraints-aws" "${file}"

	echo "Deploy 5 machines with different constraints"
  juju add-machine --constraints "root-disk=16G"
  juju add-machine --constraints "cores=1"
  juju add-machine --constraints "cpu-power=30"
  juju add-machine --constraints "root-disk=16G cpu-power=30"
  juju add-machine --constraints "instance-type=i3.xlarge"

  wait_for_machine_agent_status "0" "started"
  wait_for_machine_agent_status "1" "started"
  wait_for_machine_agent_status "2" "started"
  wait_for_machine_agent_status "3" "started"
  wait_for_machine_agent_status "4" "started"

	echo "Ensure machine 0 has 16G root disk"
  machine0_hardware=$(juju machines --format json | jq -r '.["machines"]["0"]["hardware"]')
  check_contains "${machine0_hardware}" "root-disk=16384M"

	echo "Ensure machine 1 has 1 core"
  machine1_hardware=$(juju machines --format json | jq -r '.["machines"]["1"]["hardware"]')
  check_contains "${machine1_hardware}" "cores=1"

	echo "Ensure machine 2 limit cpu-power by 30"
  machine2_constraints=$(juju machines --format json | jq -r '.["machines"]["2"]["constraints"]')
  check_contains "${machine2_constraints}" "cpu-power=30"

	echo "Ensure machine 3 has 16G root disk and limit of cpu power by 30"
  machine3_hardware=$(juju machines --format json | jq -r '.["machines"]["3"]["hardware"]')
  machine3_constraints=$(juju machines --format json | jq -r '.["machines"]["3"]["constraints"]')
  check_contains "${machine3_hardware}" "root-disk=16384M"
  check_contains "${machine3_constraints}" "cpu-power=30"

	echo "Ensure machine 4 has i3.xlarge instance type"
  machine4_constraints=$(juju machines --format json | jq -r '.["machines"]["4"]["constraints"]')
  check_contains "${machine4_constraints}" "instance-type=i3.xlarge"

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