test_spaces_ec2() {
    if [ "$(skip 'test_spaces_ec2')" ]; then
        echo "==> TEST SKIPPED: space tests (EC2)"
        return
    fi

    set_verbosity

    echo "==> Checking for dependencies"
    check_dependencies juju aws

    echo "==> Setting up additional EC2 resources for tests"
    hotplug_nic_id=$(setup_nic_for_space_tests)
    echo "[+] created secondary NIC with ID ${hotplug_nic_id}"
    function remove_nic() {
      aws ec2 delete-network-interface --network-interface-id "$hotplug_nic_id"
    }
    trap remove_nic EXIT

    file="${TEST_DIR}/test-spaces-ec2.txt"

    bootstrap "test-spaces-ec2" "${file}"

    test_upgrade_charm_with_bind "$hotplug_nic_id"
    test_juju_bind "$hotplug_nic_id"

    destroy_controller "test-spaces-ec2"
}

setup_nic_for_space_tests() {
  isolated_subnet_id=$(aws ec2 describe-subnets --filters Name=cidr-block,Values=172.31.254.0/24 2>/dev/null | jq -r '.Subnets[0].SubnetId')
  if [ -z "$isolated_subnet_id" ]; then
      # shellcheck disable=SC2046
      echo $(red "To run these tests you need to create a subnet with name \"isolated\" and CIDR \"172.31.254/24\"")
      exit 1
  fi

  hotplug_nic_id=$(aws ec2 create-network-interface --subnet-id "$isolated_subnet_id" --description="hot-pluggable NIC for space tests" 2>/dev/null | jq -r '.NetworkInterface.NetworkInterfaceId')
  if [ -z "$hotplug_nic_id" ]; then
      # shellcheck disable=SC2046
      echo $(red "Unable to create extra NIC for space tests; please check that your account has permissions to create NICs")
      exit 1
  fi

  # Add a created_at tag so we can use a background job to clean forgotten NICs
  aws ec2 create-tags --resources "$hotplug_nic_id" --tags Key=created_at,Value=$(date +"%Y-%m-%d")

  echo "$hotplug_nic_id"
}
