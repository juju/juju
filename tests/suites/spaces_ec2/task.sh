test_spaces_ec2() {
	if [ "$(skip 'test_spaces_ec2')" ]; then
		echo "==> TEST SKIPPED: space tests (EC2)"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju aws

	echo "==> Checking for stale NIC resources"
	cleanup_stale_nics

	echo "==> Setting up additional EC2 resources for tests"
	hotplug_nic_id=$(setup_nic_for_space_tests)
	echo "[+] created secondary NIC with ID ${hotplug_nic_id}"

	file="${TEST_DIR}/test-spaces-ec2.log"

	bootstrap "test-spaces-ec2" "${file}"

	test_upgrade_charm_with_bind "$hotplug_nic_id"
	test_juju_bind "$hotplug_nic_id"

	destroy_controller "test-spaces-ec2"

	# We don't really care if this fails; stale NICs will be auto-purged the
	# next time we run the test-suite
	# shellcheck disable=SC2086
	aws ec2 delete-network-interface --network-interface-id "$hotplug_nic_id" 2>/dev/null
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
	aws ec2 create-tags --resources "$hotplug_nic_id" --tags Key=created_at,Value="$(date +'%Y-%m-%d')"

	echo "$hotplug_nic_id"
}

cleanup_stale_nics() {
	# Each NIC is tagged with a created_at key and the "YYYY-MM-DD" value
	# corresponding to the NIC creation date. Since we don't know whether
	# other tests are running concurrently, we simply iterate the list of
	# custom NIC interfaces, filter *out* the ones with today's date and
	# try to delete anything older in a best effort manner.
	aws ec2 describe-network-interfaces --filter Name=description,Values="hot-pluggable NIC for space tests" |
		grep 'NetworkInterfaceId\|Value' |
		cut -d'"' -f4 |
		paste - - -d, |
		grep -v "$(date +'%Y-%m-%d')" |
		cut -d',' -f 1 |
		xargs -I % echo 'echo "[!] attempting to remove stale NIC % (will retry later if currently in-use)"; aws ec2 delete-network-interface --network-interface-id % 2>/dev/null' |
		sh ||
		true
}
