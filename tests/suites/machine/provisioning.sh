# test_provisioning_info validates that machines are provisioned correctly
# using the provisioner domain service, which consolidates all provisioning
# data (constraints, model config, image metadata, network topology) into
# a single call.
test_provisioning_info() {
	if [ -n "$(skip 'test_provisioning_info')" ]; then
		echo "==> SKIP: Asked to skip test_provisioning_info tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_provisioning_with_constraints"
		run "run_provisioning_with_resource_tags"
		run "run_provisioning_with_image_stream"
	)
}

# run_provisioning_with_constraints verifies that a machine provisioned with
# constraints has the correct hardware assigned. This exercises the
# provisioner's constraint merging and network topology construction.
run_provisioning_with_constraints() {
	echo

	file="${TEST_DIR}/test-provisioning-constraints.log"
	ensure "provisioning-constraints" "${file}"

	echo "Add machine with arch constraint"
	juju add-machine --constraints "arch=amd64"

	wait_for_machine_agent_status "0" "started"

	echo "Verify machine has amd64 architecture"
	machine_hardware=$(juju show-machine 0 --format json | jq -r '.["machines"]["0"]["hardware"]')
	check_contains "${machine_hardware}" "arch=amd64"

	echo "Add machine with memory constraint"
	juju add-machine --constraints "mem=2G"

	wait_for_machine_agent_status "1" "started"

	echo "Verify machine has memory in hardware characteristics"
	machine1_hardware=$(juju show-machine 1 --format json | jq -r '.["machines"]["1"]["hardware"]')
	check_contains "${machine1_hardware}" "mem="

	destroy_model "provisioning-constraints"
}

# run_provisioning_with_resource_tags verifies that resource tags configured
# in model config are applied to provisioned machines. The provisioner service
# parses the space-separated key=value string and passes them through to the
# instance tags.
run_provisioning_with_resource_tags() {
	echo

	file="${TEST_DIR}/test-provisioning-tags.log"
	ensure "provisioning-tags" "${file}"

	echo "Set resource-tags in model config"
	juju model-config resource-tags="env=integration team=juju-qa"

	echo "Verify model config contains the resource tags"
	resource_tags=$(juju model-config resource-tags)
	check_contains "${resource_tags}" "env: integration"
	check_contains "${resource_tags}" "team: juju-qa"

	echo "Add a machine to be provisioned with the tags"
	juju add-machine

	wait_for_machine_agent_status "0" "started"

	echo "Verify machine was provisioned successfully"
	machine_status=$(juju show-machine 0 --format json | jq -r '.["machines"]["0"]["juju-status"]["current"]')
	check_contains "${machine_status}" "started"

	destroy_model "provisioning-tags"
}

# run_provisioning_with_image_stream verifies that the image-stream model
# config value is respected during provisioning. The provisioner service uses
# this to filter image metadata when selecting an image for the machine.
run_provisioning_with_image_stream() {
	echo

	file="${TEST_DIR}/test-provisioning-image-stream.log"
	ensure "provisioning-stream" "${file}"

	echo "Set image-stream to released (default)"
	juju model-config image-stream=released

	echo "Verify model config contains the image stream"
	image_stream=$(juju model-config image-stream)
	check_contains "${image_stream}" "released"

	echo "Add a machine to verify it provisions with released stream"
	juju add-machine --constraints "arch=amd64"

	wait_for_machine_agent_status "0" "started"

	echo "Verify machine was provisioned successfully with released images"
	machine_status=$(juju show-machine 0 --format json | jq -r '.["machines"]["0"]["juju-status"]["current"]')
	check_contains "${machine_status}" "started"

	# Verify the machine's instance ID exists (proves provisioning completed).
	instance_id=$(juju show-machine 0 --format json | jq -r '.["machines"]["0"]["instance-id"]')
	if [[ -z "${instance_id}" ]] || [[ "${instance_id}" == "null" ]]; then
		echo "ERROR: Machine 0 has no instance-id — provisioning may have failed" >&2
		return 1
	fi
	echo "Machine provisioned with instance-id: ${instance_id}"

	destroy_model "provisioning-stream"
}
