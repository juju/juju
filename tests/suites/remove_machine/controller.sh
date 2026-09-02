wait_for_controller_machine_count() {
	local expected max_attempts attempt started
	local machine_info controller_info status_info controller_refs controller_units
	local controller_ids machine_id machine_status

	expected=${1}
	max_attempts=${2:-25}
	attempt=0

	while true; do
		machine_info="$(juju machines -m controller --format=json)"
		controller_info="$(juju show-controller --format=json)"
		status_info="$(juju status -m controller --format=json)"
		controller_ids="$(yq -r '.[] | (.["controller-machines"] // {}) | keys | .[]' <<<"${controller_info}")"
		started=0
		while IFS= read -r machine_id; do
			if [[ -z ${machine_id} ]]; then
				continue
			fi
			machine_status="$(id="${machine_id}" yq -r \
				'.machines[env(id)]."juju-status".current' <<<"${machine_info}")"
			if [[ ${machine_status} == "started" ]]; then
				started=$((started + 1))
			fi
		done <<<"${controller_ids}"
		controller_refs="$(yq -r '.[] | (.["controller-machines"] // {}) | length' <<<"${controller_info}")"
		controller_units="$(yq -r '(.applications.controller.units // {}) | length' <<<"${status_info}")"
		if [[ ${started} == "${expected}" &&
			${controller_refs} == "${expected}" &&
			${controller_units} == "${expected}" ]]; then
			break
		fi

		echo "[+] (attempt ${attempt}) waiting for ${expected} started controller machine(s)"
		juju machines -m controller 2>&1 | sed 's/^/    | /g' || true
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))
		if [[ ${attempt} -gt ${max_attempts} ]]; then
			echo "remove-machine failed waiting for ${expected} controller machine(s)"
			exit 1
		fi
	done
}

add_machine_id() {
	local output machine_id

	output=$(juju add-machine "$@" 2>&1)
	printf '%s\n' "${output}" >&2
	machine_id=$(sed -En \
		's/^created (machine|container) ([^[:space:]]+)$/\2/p' <<<"${output}")
	if [[ -z ${machine_id} ]]; then
		echo "could not determine the new machine or container id" >&2
		return 1
	fi
	printf '%s\n' "${machine_id}"
}

wait_for_controller_leader() {
	# With one controller left, leadership is the signal that the dqlite lease
	# backstop workflow has completed and the controller is usable again.
	wait_for_or_fail "timeout 5 juju exec -m controller --unit controller/leader uptime | grep -q load" 120
}

controller_machine_ids() {
	juju show-controller --format=json |
		yq -r '.[] | (.["controller-machines"] // {}) | keys | sort_by(tonumber) | .[]'
}

controller_machine_count() {
	juju show-controller --format=json |
		yq -r '.[] | (.["controller-machines"] // {}) | length'
}

wait_for_controller_units_idle() {
	local expected status_json unit_indexes unit_index

	expected=${1}
	wait_for "${expected}" '(.applications.controller.units // {}) | length'
	status_json=$(juju status -m controller --format=json)
	unit_indexes=$(yq -r '(.applications.controller.units // {}) | keys | map(split("/")[1]) | sort_by(tonumber) | .[]' <<<"${status_json}")
	if [[ -z ${unit_indexes} ]]; then
		echo "controller units not found"
		return 1
	fi
	while IFS= read -r unit_index; do
		wait_for "controller" "$(idle_condition "controller" 0 "${unit_index}")"
	done <<<"${unit_indexes}"
}

prepare_controller_for_removal() {
	juju enable-ha
	wait_for_controller_machine_count 3 200
	wait_for_ha 3

	juju switch controller
	wait_for_controller_units_idle 3
}

assert_controller_instance_ids() {
	juju show-controller --format=json |
		yq -r '[.[] | .["controller-machines"][] | select(.["instance-id"] == null)] | length' |
		check 0
}

run_remove_controller_machine() {
	local file machine_id expected

	echo

	file="${TEST_DIR}/remove_controller_machine.log"
	ensure "remove-controller-machine" "${file}"

	prepare_controller_for_removal
	expected=$(controller_machine_count)
	while [[ ${expected} -gt 1 ]]; do
		machine_id=$(controller_machine_ids | tail -n 1)
		juju remove-machine -m controller "${machine_id}" --no-prompt
		expected=$((expected - 1))
		wait_for_controller_machine_count "${expected}"
		wait_for_machine_removed "${machine_id}"
	done

	assert_controller_instance_ids
	wait_for_controller_leader

	destroy_model "remove-controller-machine"
}

deploy_related_controller_units() {
	local controller_machine_id sink_machine_id token

	controller_machine_id=${1}
	sink_machine_id=${2}
	juju deploy ./testcharms/charms/dummy-source --to "${controller_machine_id}"
	juju deploy ./testcharms/charms/dummy-sink --to "${sink_machine_id}"
	juju integrate dummy-source dummy-sink
	token=$(rnd_str)
	juju config dummy-source token="${token}"
	wait_for "idle" '(.applications."dummy-source".units // {})[] | ."juju-status".current'
	wait_for "active" '(.applications."dummy-source".units // {})[] | ."workload-status".current'
	wait_for "idle" '(.applications."dummy-sink".units // {})[] | ."juju-status".current'
	wait_for "active" '(.applications."dummy-sink".units // {})[] | ."workload-status".current'
}

remove_related_controller_applications() {
	local application

	for application in dummy-source dummy-sink; do
		juju remove-application -m controller "${application}" --no-prompt
		wait_for "false" ".applications | has(\"${application}\")"
	done
}

run_remove_controller_machine_with_units() {
	local file controller_machine_id workload_machine_id

	echo

	file="${TEST_DIR}/remove_controller_machine_with_units.log"
	ensure "remove-controller-machine-with-units" "${file}"

	prepare_controller_for_removal

	# Keep one relation endpoint on a separate machine so its departed hook
	# records the removal of the unit hosted by the controller.
	juju switch controller
	controller_machine_id=$(controller_machine_ids | tail -n 1)
	workload_machine_id=$(add_machine_id)
	wait_for_machine_agent_status "${workload_machine_id}" "started"
	deploy_related_controller_units "${controller_machine_id}" "${workload_machine_id}"

	juju remove-machine -m controller "${controller_machine_id}" --no-prompt
	wait_for_controller_machine_count 2
	wait_for_machine_removed "${controller_machine_id}"
	wait_for "0" '.applications."dummy-source".units // {} | length'
	wait_for "source relation departed" \
		'(.applications."dummy-sink".units // {})[] | ."workload-status".message'
	remove_related_controller_applications
	wait_for_machine_removed "${workload_machine_id}"

	destroy_model "remove-controller-machine-with-units"
}

run_force_remove_controller_machine_with_units() {
	local file controller_machine_id workload_machine_id

	echo

	file="${TEST_DIR}/force_remove_controller_machine_with_units.log"
	ensure "force-remove-controller-machine-with-units" "${file}"

	prepare_controller_for_removal

	juju switch controller
	controller_machine_id=$(controller_machine_ids | tail -n 1)
	workload_machine_id=$(add_machine_id)
	wait_for_machine_agent_status "${workload_machine_id}" "started"
	deploy_related_controller_units "${controller_machine_id}" "${workload_machine_id}"

	juju remove-machine -m controller "${controller_machine_id}" --force --no-wait --no-prompt
	wait_for_controller_machine_count 2
	wait_for_machine_removed "${controller_machine_id}"
	wait_for "0" '.applications."dummy-source".units // {} | length'
	assert_controller_instance_ids
	remove_related_controller_applications
	wait_for_machine_removed "${workload_machine_id}"

	destroy_model "force-remove-controller-machine-with-units"
}

remove_controller_machine_with_containers() {
	local force test_name file controller_machine_id container_id

	force=${1}
	if [[ ${force} == "true" ]]; then
		test_name="force-remove-controller-machine-with-containers"
		file="${TEST_DIR}/force_remove_controller_machine_with_containers.log"
	else
		test_name="remove-controller-machine-with-containers"
		file="${TEST_DIR}/remove_controller_machine_with_containers.log"
	fi

	echo

	ensure "${test_name}" "${file}"

	prepare_controller_for_removal

	juju switch controller
	controller_machine_id=$(controller_machine_ids | tail -n 1)
	container_id=$(add_machine_id "lxd:${controller_machine_id}")
	wait_for_container_agent_status "${container_id}" "started"
	deploy_related_controller_units "${controller_machine_id}" "${container_id}"

	if [[ ${force} == "true" ]]; then
		juju remove-machine -m controller "${controller_machine_id}" --force --no-wait --no-prompt
	else
		juju remove-machine -m controller "${controller_machine_id}" --no-prompt
	fi
	wait_for_controller_machine_count 2
	wait_for_machine_removed "${controller_machine_id}"
	wait_for "0" '.applications."dummy-source".units // {} | length'
	wait_for "0" '.applications."dummy-sink".units // {} | length'
	assert_controller_instance_ids
	remove_related_controller_applications

	destroy_model "${test_name}"
}

run_remove_controller_machine_with_containers() {
	remove_controller_machine_with_containers false
}

run_force_remove_controller_machine_with_containers() {
	remove_controller_machine_with_containers true
}

run_force_remove_controller_machine() {
	local file machine_id

	echo

	file="${TEST_DIR}/force_remove_controller_machine.log"
	ensure "force-remove-controller-machine" "${file}"

	prepare_controller_for_removal
	machine_id=$(controller_machine_ids | tail -n 1)
	delete_cloud_instance "controller" "${machine_id}"
	juju remove-machine -m controller "${machine_id}" --force --no-wait --no-prompt
	wait_for_controller_machine_count 2
	wait_for_machine_removed "${machine_id}"
	assert_controller_instance_ids

	destroy_model "force-remove-controller-machine"
}

test_remove_controller_machine() {
	if [ -n "$(skip 'test_remove_controller_machine')" ]; then
		echo "==> SKIP: Asked to skip controller remove-machine tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		if cloud_instance_removal_supported; then
			run "run_force_remove_controller_machine"
		else
			echo "==> TEST SKIPPED: force remove controller machine - cloud instance removal is unsupported"
		fi
		run "run_remove_controller_machine"
		run "run_remove_controller_machine_with_units"
		run "run_force_remove_controller_machine_with_units"

		case "${BOOTSTRAP_PROVIDER:-}" in
		"lxd")
			if stat /dev/kvm; then
				run "run_remove_controller_machine_with_containers"
				run "run_force_remove_controller_machine_with_containers"
			else
				echo "==> TEST SKIPPED: remove_controller_machine_with_containers - lxd without kvm is not supported"
				echo "==> TEST SKIPPED: force_remove_controller_machine_with_containers - lxd without kvm is not supported"
			fi
			;;
		*)
			run "run_remove_controller_machine_with_containers"
			run "run_force_remove_controller_machine_with_containers"
			;;
		esac
	)
}
