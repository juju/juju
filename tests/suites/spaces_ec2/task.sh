test_spaces_ec2() {
    if [ "$(skip 'test_spaces_ec2')" ]; then
        echo "==> TEST SKIPPED: space tests (EC2)"
        return
    fi

    set_verbosity

    echo "==> Checking for dependencies"
    check_dependencies juju aws

    echo "==> Checking for EC2 prerequisites"
    check_prerequisites_for_space_tests

    file="${TEST_DIR}/test-spaces-ec2.txt"

    bootstrap "test-spaces-ec2" "${file}"

    test_upgrade_charm_with_bind
    test_juju_bind

    destroy_controller "test-spaces-ec2"
}

check_prerequisites_for_space_tests() {
  match_count=$(aws ec2 describe-subnets --filters Name=cidr-block,Values=172.31.254.0/24 | jq '.Subnets | length')
  if [[ "$match_count" != "1" ]]; then
      # shellcheck disable=SC2046
      echo $(red "To run these tests you need to create a subnet with name \"isolated\" and CIDR \"172.31.254/24\"")
      exit 1
  fi

  if_count=$(aws ec2 describe-network-interfaces --filters Name=tag:nic-type,Values=hotpluggable | jq '.NetworkInterfaces[] | select(.["PrivateIpAddress"] | startswith("172.31.254.")) | .NetworkInterfaceId')
  if [[ -z "$if_count" ]]; then
      # shellcheck disable=SC2046
      echo $(red "To run these tests you need to create a network interface on the isolated subnet (CIDR \"172.31.254/24\") and tag it with \"nic-type:hotpluggable\"")
      exit 1
  fi
}
