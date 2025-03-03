test_spaces_ec2() {
	if [ "$(skip 'test_spaces_ec2')" ]; then
		echo "==> TEST SKIPPED: space tests (EC2)"
		return
	fi

	setup_awscli_credential
	# Ensure that the aws cli and juju both use the same aws region
	export AWS_DEFAULT_REGION="${BOOTSTRAP_REGION}"

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju aws

	echo "==> Ensure subnet for alternative space exists"
	subnet_id=$(ensure_subnet)

	echo "==> Setting up additional EC2 resources for tests"
	hotplug_nic_id=$(setup_nic_for_space_tests "${subnet_id}")
	echo "[+] created secondary NIC with ID ${hotplug_nic_id}"

	file="${TEST_DIR}/test-spaces-ec2.log"

	bootstrap "test-spaces-ec2" "${file}"

	test_machines_in_spaces
	test_upgrade_charm_with_bind "$hotplug_nic_id"
	test_juju_bind "$hotplug_nic_id"

	destroy_controller "test-spaces-ec2"

	# We don't really care if this fails; stale NICs will be auto-purged the
	# next time we run the test-suite
	# shellcheck disable=SC2086
	aws ec2 delete-network-interface --network-interface-id "$hotplug_nic_id" 2>/dev/null || true
}

ensure_subnet() {
	isolated_subnet_id=$(aws ec2 describe-subnets --filters Name=cidr-block,Values=172.31.254.0/24 2>/dev/null | jq -r '.Subnets[0].SubnetId')
	if [ "$isolated_subnet_id" != "null" ]; then
		cleanup_stale_nics
		echo "$isolated_subnet_id"
		return
	fi

	# Create a subnet in the default vpc
	vpc_id=$(aws ec2 describe-vpcs | jq -r ".Vpcs[0].VpcId")
	subnet_id=$(aws ec2 create-subnet --vpc-id "${vpc_id}" --cidr-block "172.31.254.0/24" | jq -r ".Subnet.SubnetId")
	if [ -z "${subnet_id}" ] || [ "${subnet_id}" == "null" ]; then
		echo "$(red "failed to create subnet in vpc $vpc_id")" 1>&2
		exit 1
	fi
	echo "${subnet_id}"
}

setup_nic_for_space_tests() {
	isolated_subnet_id=${1}
	hotplug_nic_id=$(aws ec2 create-network-interface --subnet-id "$isolated_subnet_id" --description="hot-pluggable NIC for space tests" 2>"${TEST_DIR}/create-network-interface-stderr.log" | jq -r '.NetworkInterface.NetworkInterfaceId')
	if [ -z "$hotplug_nic_id" ] || [ "$hotplug_nic_id" == "null" ]; then
		# shellcheck disable=SC2046
		echo $(red "Unable to create extra NIC for space tests; please check that your account has permissions to create NICs. Failed with:") 1>&2
		cat "${TEST_DIR}/create-network-interface-stderr.log" 1>&2
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

	# TODO(jack-w-shaw) fix this. This:
	# 1) Should use jq
	# 2) Should work. At the moment the created_at tag is not created, so all nics are destroyed
	aws ec2 describe-network-interfaces --filter Name=description,Values="hot-pluggable NIC for space tests" |
		grep 'NetworkInterfaceId\|Value' |
		cut -d'"' -f4 |
		paste - - -d, |
		grep -v "$(date +'%Y-%m-%d')" |
		cut -d',' -f 1 |
		xargs -I % echo 'echo "[!] attempting to remove stale NIC % (will retry later if currently in-use)" 1>&2; aws ec2 delete-network-interface --network-interface-id % 2>/dev/null' |
		sh ||
		true
}
