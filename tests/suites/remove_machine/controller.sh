wait_for_controller_machine_count() {
	local expected attempt machine_info total started stopped errors

	expected=${1}
	attempt=0

	while true; do
		machine_info="$(juju machines -m controller --format=json)"
		total="$(yq -r '.machines | length' <<<"${machine_info}")"
		started="$(yq -r '[.machines[] | select(.["juju-status"].current == "started")] | length' <<<"${machine_info}")"
		stopped="$(yq -r '[.machines[] | select(.["juju-status"].current == "stopped")] | length' <<<"${machine_info}")"
		errors="$(yq -r '[.machines[] | select(.["juju-status"].current == "error")] | length' <<<"${machine_info}")"
		if [[ ${total} == "${expected}" && ${started} == "${expected}" && ${stopped} == "0" && ${errors} == "0" ]]; then
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

run_remove_controller_machine() {
	echo

	file="${TEST_DIR}/remove_controller_machine.log"
	ensure "remove-controller-machine" "${file}"

	juju enable-ha
	wait_for_controller_machines 3
	wait_for_ha 3

	juju switch controller
	wait_for "controller" "$(idle_condition "controller" 0 0)"
	wait_for "controller" "$(idle_condition "controller" 0 1)"
	wait_for "controller" "$(idle_condition "controller" 0 2)"

	if [[ ${BOOTSTRAP_PROVIDER:-} == "lxd" ]]; then
		check_dependencies lxc
		instance_id="$(juju show-machine -m controller 2 --format=json | yq -r '.machines."2"."instance-id"')"
		lxc info "${instance_id}" >/dev/null
		lxc delete --force "${instance_id}"
		juju remove-machine -m controller 2 --force --no-wait --no-prompt
	else
		juju remove-machine -m controller 2 --force --no-prompt
	fi
	wait_for_controller_machine_count 2
	juju remove-machine -m controller 1 --no-prompt
	wait_for_controller_machine_count 1

	juju show-controller --format=json |
		yq -r '[.[] | .["controller-machines"][] | select(.["instance-id"] == null)] | length' |
		check 0
	wait_for_controller_leader

	juju switch remove-controller-machine
	destroy_model "remove-controller-machine"
}

test_remove_controller_machine() {
	if [ -n "$(skip 'test_remove_controller_machine')" ]; then
		echo "==> SKIP: Asked to skip controller remove-machine tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_remove_controller_machine"
	)
}
