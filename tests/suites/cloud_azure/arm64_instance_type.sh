run_arm64_instance_type() {
	echo

	local name file hardware
	name="azure-arm64-instance-type"
	file="${TEST_DIR}/test-azure-arm64-instance-type.log"
	ensure "${name}" "${file}"

	# Standard_E8pds_v6 is an Arm64 size that is not in the Arm64 family list.
	# Juju used to read it as amd64 and reject these constraints as ambiguous.
	# See https://github.com/juju/juju/issues/23835.
	juju add-machine --base ubuntu@24.04 --constraints "arch=arm64 instance-type=Standard_E8pds_v6"
	wait_for_machine_agent_status "0" "started"

	hardware=$(juju show-machine 0 --format json | yq -r '.machines["0"].hardware')
	check_contains "${hardware}" "arch=arm64"

	destroy_model "${name}"
}

test_arm64_instance_type() {
	if [ "$(skip 'test_arm64_instance_type')" ]; then
		echo "==> TEST SKIPPED: azure arm64 instance type"
		return
	fi

	if [ "$(az account list | yq -r 'length')" -lt 1 ]; then
		echo "==> TEST SKIPPED: not logged in to Azure cloud"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_arm64_instance_type" "$@"
	)
}
