wait_for_machine_removed() {
	local machine_id

	machine_id=${1}

	wait_for "false" ".machines | has(\"${machine_id}\")"
}

add_container_host_machine() {
	# Container-in-container is unreliable on the LXD provider, so use a VM
	# for the container host.
	if [[ ${BOOTSTRAP_PROVIDER:-} == "lxd" ]]; then
		juju add-machine "$@" --constraints="virt-type=virtual-machine"
	else
		juju add-machine "$@"
	fi
}

run_remove_machine_without_workloads() {
	local file

	echo

	file="${TEST_DIR}/remove_machine_without_workloads.log"
	ensure "remove-machine-empty" "${file}"

	juju add-machine
	wait_for_machine_agent_status "0" "started"

	juju remove-machine 0
	wait_for_machine_removed "0"

	destroy_model "remove-machine-empty"
}

run_remove_machine_with_unit() {
	local file status_json token source_machine_id

	echo

	file="${TEST_DIR}/remove_machine_with_unit.log"
	ensure "remove-machine-unit" "${file}"

	juju deploy ./testcharms/charms/dummy-source
	juju deploy ./testcharms/charms/dummy-sink
	juju integrate dummy-source dummy-sink
	token=$(rnd_str)
	juju config dummy-source token="${token}"
	wait_for "idle" '(.applications."dummy-source".units // {})[] | ."juju-status".current'
	wait_for "active" '(.applications."dummy-source".units // {})[] | ."workload-status".current'
	wait_for "idle" '(.applications."dummy-sink".units // {})[] | ."juju-status".current'
	wait_for "active" '(.applications."dummy-sink".units // {})[] | ."workload-status".current'

	source_machine_id=$(juju status dummy-source --format=json |
		yq -r '(.applications."dummy-source".units // {})[].machine')

	juju remove-machine "${source_machine_id}" --dry-run
	status_json=$(juju status --format=json)
	yq -r ".machines | has(\"${source_machine_id}\")" <<<"${status_json}" | check true
	yq -r '.applications."dummy-source".units // {} | length' <<<"${status_json}" | check 1

	juju remove-machine "${source_machine_id}" --no-prompt
	wait_for_machine_removed "${source_machine_id}"
	wait_for "0" '.applications."dummy-source".units // {} | length'
	wait_for "source relation departed" \
		'(.applications."dummy-sink".units // {})[] | ."workload-status".message'

	destroy_model "remove-machine-unit"
}

run_force_remove_machine_with_unit_without_instance() {
	local file target_machine_id

	echo

	file="${TEST_DIR}/force_remove_machine_with_unit_without_instance.log"
	ensure "force-remove-machine" "${file}"

	juju deploy ubuntu-lite
	wait_for "idle" '(.applications."ubuntu-lite".units // {})[] | ."juju-status".current'
	wait_for "active" '(.applications."ubuntu-lite".units // {})[] | ."workload-status".current'
	target_machine_id=$(juju status ubuntu-lite --format=json |
		yq -r '(.applications."ubuntu-lite".units // {})[].machine')
	delete_cloud_instance "force-remove-machine" "${target_machine_id}"
	juju remove-machine "${target_machine_id}" --force --no-prompt
	wait_for_machine_removed "${target_machine_id}"
	wait_for "0" '.applications."ubuntu-lite".units // {} | length'

	destroy_model "force-remove-machine"
}

run_remove_machine_with_parent_and_container_units() {
	local file

	echo

	file="${TEST_DIR}/remove_machine_with_parent_and_container_units.log"
	ensure "remove-machine-container-unit" "${file}"

	add_container_host_machine --base ubuntu@20.04
	wait_for_machine_agent_status "0" "started"
	juju add-machine lxd:0 --base ubuntu@20.04
	wait_for_container_agent_status "0/lxd/0" "started"
	juju deploy ubuntu-lite -n 2 --base ubuntu@20.04 --to 0,0/lxd/0
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite" 0 0)"
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite" 0 1)"

	juju remove-machine 0 --no-prompt
	wait_for_machine_removed "0"
	wait_for "0" '.applications."ubuntu-lite".units // {} | length'

	destroy_model "remove-machine-container-unit"
}

run_force_remove_machine_with_container() {
	local file

	echo

	file="${TEST_DIR}/force_remove_machine_with_container.log"
	ensure "remove-machine-container-force" "${file}"

	add_container_host_machine
	wait_for_machine_agent_status "0" "started"
	juju add-machine lxd:0
	wait_for_container_agent_status "0/lxd/0" "started"

	juju remove-machine 0 --force --no-prompt
	wait_for_machine_removed "0"

	destroy_model "remove-machine-container-force"
}

test_remove_non_controller_machines() {
	if [ -n "$(skip 'test_remove_non_controller_machines')" ]; then
		echo "==> SKIP: Asked to skip non-controller remove-machine tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_remove_machine_without_workloads"
		run "run_remove_machine_with_unit"
		if cloud_instance_removal_supported; then
			run "run_force_remove_machine_with_unit_without_instance"
		else
			echo "==> TEST SKIPPED: force remove machine - cloud instance removal is unsupported"
		fi

		case "${BOOTSTRAP_PROVIDER:-}" in
		"lxd")
			if stat /dev/kvm; then
				run "run_remove_machine_with_parent_and_container_units"
				run "run_force_remove_machine_with_container"
			else
				echo "==> TEST SKIPPED: remove_machine_with_parent_and_container_units - lxd without kvm is not supported"
				echo "==> TEST SKIPPED: force_remove_machine_with_container - lxd without kvm is not supported"
			fi
			;;
		*)
			run "run_remove_machine_with_parent_and_container_units"
			run "run_force_remove_machine_with_container"
			;;
		esac
	)
}
