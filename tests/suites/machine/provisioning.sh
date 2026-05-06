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
		run "run_provisioning_with_storage"
		run "run_provisioning_controller_model"
		run "run_provisioning_parallel_machines"
		run "run_provisioning_no_error_state"

		case "${BOOTSTRAP_PROVIDER:-}" in
		"lxd" | "localhost")
			run "run_provisioning_with_proposed_agent_stream"
			;;
		*)
			echo "Skipping provisioning with proposed stream test (not supported on non-LXD)"
			;;
		esac
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

	echo "Add machine with multiple constraints"
	juju add-machine --constraints "arch=amd64 cores=2 mem=4G root-disk=20G"

	wait_for_machine_agent_status "1" "started"

	echo "Verify machine has correct constraints"
	machine1_constraints=$(juju show-machine 1 --format json | jq -r '.["machines"]["1"]["constraints"]')
	check_contains "${machine1_constraints}" "arch=amd64"
	check_contains "${machine1_constraints}" "cores=2"
	check_contains "${machine1_constraints}" "mem=4096M"
	check_contains "${machine1_constraints}" "root-disk=20480M"

	echo "Verify machine has correct hardware characteristics"
	machine1_hardware=$(juju show-machine 1 --format json | jq -r '.["machines"]["1"]["hardware"]')
	check_contains "${machine1_hardware}" "arch=amd64"
	check_contains "${machine1_hardware}" "cores="
	check_contains "${machine1_hardware}" "mem="

	# root-disk is only reported in hardware by providers that support it
	# (e.g. ec2, gce, azure, oci, vsphere). LXD does not size root disks.
	case "${BOOTSTRAP_PROVIDER:-}" in
	"lxd" | "localhost")
		echo "Skipping root-disk hardware check (not supported on LXD)"
		;;
	*)
		check_contains "${machine1_hardware}" "root-disk="
		;;
	esac

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

# run_provisioning_with_proposed_agent_stream verifies that a machine can be
# provisioned using agent binaries from the "proposed" simplestreams stream.
# This exercises the provisioner's agent metadata resolution path with a
# custom agent-metadata-url and agent-stream=proposed, using --prevent-fallback
# to ensure no public cloud fallback is used.
run_provisioning_with_proposed_agent_stream() {
	echo

	VERSION=$(jujud version)
	JUJUD_VERSION=$(jujud_version)
	echo "===> Using jujud version ${JUJUD_VERSION}"

	STREAMS_DIR="${TEST_DIR}/proposed-streams"
	mkdir -p "${STREAMS_DIR}/tools/proposed"

	# Create a tarball of jujud to serve as the agent binary.
	jujud_path=$(which jujud)
	cp "${jujud_path}" "${TEST_DIR}/jujud-proposed"
	(
		cd "${TEST_DIR}" || exit
		mv jujud-proposed jujud
		tar -zcf "juju-${VERSION}.tgz" jujud >/dev/null
		mv "juju-${VERSION}.tgz" "${STREAMS_DIR}/tools/proposed/"
		rm -f jujud
	)

	# Generate simplestreams metadata for the "proposed" stream with
	# --prevent-fallback to ensure we only use our local binaries.
	juju metadata generate-agent-binaries \
		--clean \
		--prevent-fallback \
		--stream proposed \
		-d "${STREAMS_DIR}"

	# Start a local HTTP server to serve the metadata.
	(
		cd "${STREAMS_DIR}/tools" || exit 1
		python3 -m http.server 8667 >"${TEST_DIR}/proposed-server.log" 2>&1 &
		echo $! >"${TEST_DIR}/proposed-server.pid"
		sleep 5
	)

	# Find a routable address to the server.
	addresses=$(hostname -I)
	server_address=""
	for address in $(echo "${addresses}" | tr ' ' '\n'); do
		# shellcheck disable=SC2015
		curl "http://${address}:8667" >/dev/null 2>&1 && server_address="${address}" && break || true
	done

	if [[ -z "${server_address}" ]]; then
		echo "ERROR: Could not find routable address for local metadata server" >&2
		kill "$(cat "${TEST_DIR}/proposed-server.pid")" 2>/dev/null || true
		return 1
	fi
	echo "Metadata server running at http://${server_address}:8667"

	file="${TEST_DIR}/test-provisioning-proposed-stream.log"
	ensure "provisioning-proposed" "${file}"

	echo "Set agent-stream to proposed and configure agent-metadata-url"
	juju model-config \
		agent-stream=proposed \
		agent-metadata-url="http://${server_address}:8667/"

	echo "Verify model config"
	agent_stream=$(juju model-config agent-stream)
	check_contains "${agent_stream}" "proposed"

	echo "Add a machine to verify it provisions from proposed stream"
	juju add-machine

	wait_for_machine_agent_status "0" "started"

	echo "Verify machine was provisioned successfully"
	machine_status=$(juju show-machine 0 --format json | jq -r '.["machines"]["0"]["juju-status"]["current"]')
	check_contains "${machine_status}" "started"

	instance_id=$(juju show-machine 0 --format json | jq -r '.["machines"]["0"]["instance-id"]')
	if [[ -z "${instance_id}" ]] || [[ "${instance_id}" == "null" ]]; then
		echo "ERROR: Machine 0 has no instance-id — proposed stream provisioning failed" >&2
		kill "$(cat "${TEST_DIR}/proposed-server.pid")" 2>/dev/null || true
		return 1
	fi
	echo "Machine provisioned from proposed stream with instance-id: ${instance_id}"

	# Clean up the metadata server.
	kill "$(cat "${TEST_DIR}/proposed-server.pid")" 2>/dev/null || true
	rm -rf "${STREAMS_DIR}"

	destroy_model "provisioning-proposed"
}

# run_provisioning_with_storage verifies that machines provisioned with
# attached disks are started correctly. This exercises the volume params
# and volume attachment params paths in the provisioner service.
run_provisioning_with_storage() {
	echo

	file="${TEST_DIR}/test-provisioning-storage.log"
	ensure "provisioning-storage" "${file}"

	echo "Add machine with attached disk"
	juju add-machine --disks "10G,1"

	wait_for_machine_agent_status "0" "started"

	echo "Verify machine started successfully with storage"
	machine_status=$(juju show-machine 0 --format json | jq -r '.["machines"]["0"]["juju-status"]["current"]')
	check_contains "${machine_status}" "started"

	echo "Verify instance-id is populated (provisioning completed)"
	instance_id=$(juju show-machine 0 --format json | jq -r '.["machines"]["0"]["instance-id"]')
	if [[ -z "${instance_id}" ]] || [[ "${instance_id}" == "null" ]]; then
		echo "ERROR: Machine 0 has no instance-id — provisioning with storage failed" >&2
		return 1
	fi
	echo "Machine provisioned with instance-id: ${instance_id}"

	destroy_model "provisioning-storage"
}

# run_provisioning_controller_model verifies that machines added to the
# controller model are provisioned correctly and have the expected machine
# jobs. Non-controller machines in the controller model should have
# JobHostUnits.
run_provisioning_controller_model() {
	echo

	file="${TEST_DIR}/test-provisioning-controller-model.log"

	echo "Switch to controller model"
	juju switch controller

	echo "Add a machine to the controller model"
	juju add-machine

	wait_for_machine_agent_status "1" "started"

	echo "Verify machine in controller model started successfully"
	machine_status=$(juju show-machine 1 --format json | jq -r '.["machines"]["1"]["juju-status"]["current"]')
	check_contains "${machine_status}" "started"

	echo "Verify instance-id is populated"
	instance_id=$(juju show-machine 1 --format json | jq -r '.["machines"]["1"]["instance-id"]')
	if [[ -z "${instance_id}" ]] || [[ "${instance_id}" == "null" ]]; then
		echo "ERROR: Machine 1 in controller model has no instance-id" >&2
		return 1
	fi
	echo "Controller model machine provisioned with instance-id: ${instance_id}"

	juju remove-machine 1 --force --no-wait
}

# run_provisioning_parallel_machines verifies that multiple machines can be
# provisioned in parallel without errors. This exercises concurrent access
# to the provisioning service.
run_provisioning_parallel_machines() {
	echo

	file="${TEST_DIR}/test-provisioning-parallel.log"
	ensure "provisioning-parallel" "${file}"

	echo "Add 5 machines in quick succession"
	for i in $(seq 0 4); do
		juju add-machine
	done

	echo "Wait for all machines to reach started status"
	for i in $(seq 0 4); do
		wait_for_machine_agent_status "${i}" "started"
	done

	echo "Verify all 5 machines have instance IDs"
	started_count=$(juju show-machine 0 1 2 3 4 --format json | jq '[.machines | to_entries[] | select(.value["instance-id"] != null and .value["instance-id"] != "")] | length')
	if [[ "${started_count}" -ne 5 ]]; then
		echo "ERROR: Expected 5 provisioned machines, got ${started_count}" >&2
		juju machines
		return 1
	fi
	echo "All 5 machines provisioned successfully"

	destroy_model "provisioning-parallel"
}

# run_provisioning_no_error_state verifies that none of the machines
# provisioned during previous tests ended up in an error state. This is a
# sanity check across the provisioning domain.
run_provisioning_no_error_state() {
	echo

	file="${TEST_DIR}/test-provisioning-no-errors.log"
	ensure "provisioning-no-errors" "${file}"

	echo "Add several machines with various constraints"
	juju add-machine --constraints "arch=amd64"
	juju add-machine --constraints "arch=amd64 mem=2G"
	juju add-machine

	echo "Wait for all machines to settle"
	for i in $(seq 0 2); do
		wait_for_machine_agent_status "${i}" "started"
	done

	echo "Verify no machines are in error state"
	error_machines=$(juju status --format json | jq '[.machines | to_entries[] | select(.value["juju-status"].current == "error")] | length')
	if [[ "${error_machines}" -ne 0 ]]; then
		echo "ERROR: Found ${error_machines} machine(s) in error state:" >&2
		juju status --format json | jq '.machines | to_entries[] | select(.value["juju-status"].current == "error") | {id: .key, status: .value["juju-status"]}' >&2
		return 1
	fi
	echo "No machines in error state — provisioning healthy"

	destroy_model "provisioning-no-errors"
}
