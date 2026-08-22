wait_for_controller_machine_count() {
	local expected attempt total started stopped errors
	local machine_info controller_info status_info controller_refs controller_units

	expected=${1}
	attempt=0

	while true; do
		machine_info="$(juju machines -m controller --format=json)"
		controller_info="$(juju show-controller --format=json)"
		status_info="$(juju status -m controller --format=json)"
		total="$(yq -r '.machines | length' <<<"${machine_info}")"
		started="$(yq -r '[.machines[] | select(.["juju-status"].current == "started")] | length' <<<"${machine_info}")"
		stopped="$(yq -r '[.machines[] | select(.["juju-status"].current == "stopped")] | length' <<<"${machine_info}")"
		errors="$(yq -r '[.machines[] | select(.["juju-status"].current == "error")] | length' <<<"${machine_info}")"
		controller_refs="$(yq -r '.[] | (.["controller-machines"] // {}) | length' <<<"${controller_info}")"
		controller_units="$(yq -r '(.applications.controller.units // {}) | length' <<<"${status_info}")"
		if [[ ${total} == "${expected}" && ${started} == "${expected}" && ${stopped} == "0" && ${errors} == "0" && ${controller_refs} == "${expected}" && ${controller_units} == "${expected}" ]]; then
			break
		fi

		echo "[+] (attempt ${attempt}) waiting for ${expected} started controller machine(s)"
		juju machines -m controller 2>&1 | sed 's/^/    | /g' || true
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))
		if [[ ${attempt} -gt 25 ]]; then
			echo "remove-machine failed waiting for ${expected} controller machine(s)"
			exit 1
		fi
	done
}

wait_for_controller_leader() {
	# With one controller left, leadership is the signal that the dqlite lease
	# backstop workflow has completed and the controller is usable again.
	wait_for_or_fail "timeout 5 juju exec -m controller --unit controller/leader uptime | grep -q load" 120
}

controller_machine_ids() {
	juju machines -m controller --format=json |
		yq -r '.machines | keys | sort_by(tonumber) | .[]'
}

controller_machine_count() {
	juju machines -m controller --format=json | yq -r '.machines | length'
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
	wait_for_controller_machines 3
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
	done

	assert_controller_instance_ids
	wait_for_controller_leader

	juju switch remove-controller-machine
	destroy_model "remove-controller-machine"
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
	assert_controller_instance_ids

	juju switch force-remove-controller-machine
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
	)
}
