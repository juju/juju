wait_for_controller_ha() {
	local voters

	voters=${1}
	case "${BOOTSTRAP_PROVIDER:-}" in
	"k8s" | "kubernetes" | "microk8s")
		# Kubernetes controller readiness is reflected by the controller
		# units reaching idle in wait_for_controller_unit_count.
		;;
	*)
		wait_for_ha "${voters}"
		;;
	esac
}

check_controller_usable() {
	local result action_status action_message attempt
	attempt=0

	# list-my-params is a lightweight action that echoes its input, exercising
	# a complete request through the controller API and back to a unit agent.
	# The agent can restart while its API addresses are refreshed after scaling;
	# this idempotent probe can be retried if that interrupts the action.
	while [[ ${attempt} -lt 5 ]]; do
		wait_for "juju-qa-action" "$(idle_condition "juju-qa-action")" 900
		result=$(juju run -m enable-ha --format=json juju-qa-action/0 list-my-params ping="pong")
		action_status=$(yq -r '.["juju-qa-action/0"].status' <<<"${result}")
		if [[ ${action_status} == "completed" ]]; then
			echo "${result}" | yq -r '.["juju-qa-action/0"].results.ping' | check 'pong'
			return
		fi

		action_message=$(yq -r '.["juju-qa-action/0"].message' <<<"${result}")
		if [[ ${action_message} != "action terminated" ]]; then
			echo "controller usability action did not complete: ${action_status}"
			echo "${result}" | yq '.'
			exit 1
		fi

		attempt=$((attempt + 1))
		if [[ ${attempt} -eq 5 ]]; then
			echo "controller usability action was repeatedly interrupted"
			echo "${result}" | yq '.'
			exit 1
		fi
		echo "controller usability action was interrupted; retrying after unit settles"
		sleep "${SHORT_TIMEOUT}"
	done
}

# run_enable_ha exercises controller HA scale-out, scale-in and failover.
#
# KNOWN FAILURE (JUJU-9740): HA scale-out for the snap-based standalone
# controller is Phase 2 and is not implemented. This test is deliberately
# left failing rather than skipped: the failure is the signal that the gap
# still exists, and all-green CI must not imply that HA works. Do not
# weaken the assertions to make it pass; once JUJU-9740 lands the test must
# pass unmodified.
run_enable_ha() {
	local base_unit unit
	local -a controller_units remaining_units

	echo
	echo "==> KNOWN FAILURE (JUJU-9740): HA scale-out for the snap-based"
	echo "    standalone controller is not implemented. This test is"
	echo "    expected to fail until Phase 2 lands."

	file="${TEST_DIR}/enable_ha.log"

	ensure "enable-ha" "${file}"
	wait_for_controller_unit_count 1
	mapfile -t controller_units < <(controller_unit_names)
	if [[ ${#controller_units[@]} -ne 1 ]]; then
		echo "expected one controller unit before HA scale test, found: ${controller_units[*]}"
		exit 1
	fi
	base_unit=${controller_units[0]}
	wait_for_controller_ha 1

	juju deploy juju-qa-action
	wait_for "juju-qa-action" "$(idle_condition "juju-qa-action")"
	check_controller_usable

	# The scale-out below is expected to fail until JUJU-9740 lands. The
	# failure is the intended signal that HA is not implemented, not a
	# flaky test.
	echo "==> Scaling the controller to 3 units (expected to fail: JUJU-9740)"
	# Discover the current unit ordinals rather than assuming the bootstrap
	# unit is still controller/0. The suite reuses this controller model and
	# unit sequence numbers are not reused after scale-in.
	juju add-unit -m controller controller -n 2
	wait_for_controller_unit_count 3
	mapfile -t controller_units < <(controller_unit_names)
	if ! printf '%s\n' "${controller_units[@]}" | grep -Fxq "${base_unit}"; then
		echo "controller unit ${base_unit} disappeared during scale out"
		exit 1
	fi
	wait_for_controller_ha 3
	check_controller_usable

	remove_controller_unit "${base_unit}"
	wait_for_controller_unit_count 2
	mapfile -t remaining_units < <(controller_unit_names)
	if printf '%s\n' "${remaining_units[@]}" | grep -Fxq "${base_unit}"; then
		echo "controller unit ${base_unit} was not removed during scale in"
		exit 1
	fi
	wait_for_controller_ha 2
	check_controller_usable

	juju add-unit -m controller controller
	wait_for_controller_unit_count 3
	mapfile -t controller_units < <(controller_unit_names)
	for unit in "${remaining_units[@]}"; do
		if ! printf '%s\n' "${controller_units[@]}" | grep -Fxq "${unit}"; then
			echo "controller unit ${unit} disappeared during scale out"
			exit 1
		fi
	done
	wait_for_controller_ha 3
	check_controller_usable

	destroy_model "enable-ha"
}

test_enable_ha() {
	if [ -n "$(skip 'test_enable_ha')" ]; then
		echo "==> SKIP: Asked to skip controller enable-ha tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_enable_ha"
	)
}
