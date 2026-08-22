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
	local file token

	echo

	file="${TEST_DIR}/remove_machine_with_unit.log"
	ensure "remove-machine-unit" "${file}"

	juju add-machine --base ubuntu@22.04
	juju add-machine --base ubuntu@22.04
	wait_for_machine_agent_status "0" "started"
	wait_for_machine_agent_status "1" "started"
	juju deploy ./testcharms/charms/dummy-source --base ubuntu@22.04 --to 0
	juju deploy ./testcharms/charms/dummy-sink --base ubuntu@22.04 --to 1
	juju integrate dummy-source dummy-sink
	token=$(rnd_str)
	juju config dummy-source token="${token}"
	wait_for "dummy-source" "$(active_idle_condition "dummy-source" 1 0)"
	wait_for "dummy-sink" "$(active_idle_condition "dummy-sink" 0 0)"

	juju remove-machine 0 --no-prompt
	wait_for_machine_removed "0"
	wait_for "0" '.applications."dummy-source".units // {} | length'
	wait_for "source relation departed" "$(workload_status "dummy-sink" 0).message"

	destroy_model "remove-machine-unit"
}

run_force_remove_machine_with_unit_without_instance() {
	local file

	echo

	file="${TEST_DIR}/force_remove_machine_with_unit_without_instance.log"
	ensure "force-remove-machine" "${file}"

	juju add-machine --base ubuntu@20.04
	wait_for_machine_agent_status "0" "started"
	juju deploy jameinel-ubuntu-lite --base ubuntu@20.04 --to 0
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite" 0 0)"
	delete_cloud_instance "force-remove-machine" "0"
	juju remove-machine 0 --force --no-prompt
	wait_for_machine_removed "0"
	wait_for "0" '.applications."ubuntu-lite".units // {} | length'

	destroy_model "force-remove-machine"
}

run_remove_machine_with_parent_and_container_units() {
	echo

	file="${TEST_DIR}/remove_machine_with_parent_and_container_units.log"
	ensure "remove-machine-container-unit" "${file}"

	add_container_host_machine --base ubuntu@20.04
	wait_for_machine_agent_status "0" "started"
	juju add-machine lxd:0 --base ubuntu@20.04
	wait_for_container_agent_status "0/lxd/0" "started"
	juju deploy jameinel-ubuntu-lite -n 2 --base ubuntu@20.04 --to 0,0/lxd/0
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite" 0 0)"
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite" 0 1)"

	juju remove-machine 0 --no-prompt
	wait_for_machine_removed "0"
	wait_for "0" '.applications."ubuntu-lite".units // {} | length'

	destroy_model "remove-machine-container-unit"
}

run_force_remove_machine_with_container() {
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
