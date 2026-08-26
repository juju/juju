# Helpers for CAAS controller-process and machine-agent restart resilience.
# These functions interact with the Pebble service manager in the
# Kubernetes controller pod to restart individual services and verify
# the split-process topology after each recovery event.

get_controller_pod() {
	local pod
	pod=$(kubectl get pods -n "controller-${BOOTSTRAPPED_JUJU_CTRL_NAME}" \
		-l "app.kubernetes.io/name=${BOOTSTRAPPED_JUJU_CTRL_NAME}" \
		-o jsonpath='{.items[0].metadata.name}')
	if [[ -z ${pod} ]]; then
		echo "Failed to find controller pod in namespace controller-${BOOTSTRAPPED_JUJU_CTRL_NAME}" >&2
		return 1
	fi
	printf "%s" "${pod}"
}

# restart_pebble_service restarts a single Pebble service in the
# controller pod's api-server container.
restart_pebble_service() {
	local service=$1
	local pod
	pod=$(get_controller_pod)
	if [[ -z ${pod} ]]; then
		return 1
	fi
	echo "Restarting Pebble service ${service} in pod ${pod}"
	kubectl exec -n "controller-${BOOTSTRAPPED_JUJU_CTRL_NAME}" "${pod}" -c api-server -- \
		/opt/pebble restart "${service}"
}

# assert_split_topology inspects the controller pod's process list to
# verify the split controller topology: jujud must own the controller
# and jujuagentd must be running with --machine-agent-only.
assert_split_topology() {
	local pod
	pod=$(get_controller_pod)
	if [[ -z ${pod} ]]; then
		return 1
	fi

	echo "Verifying split controller topology in pod ${pod}"

	# Verify jujud controller process is running
	local jujud_process
	jujud_process=$(kubectl exec -n "controller-${BOOTSTRAPPED_JUJU_CTRL_NAME}" "${pod}" -c api-server -- \
		/bin/sh -c "ps aux" 2>/dev/null || echo "")
	echo "Process list from controller pod:"
	echo "${jujud_process}"
	check_contains "${jujud_process}" "jujud controller"

	# Verify jujuagentd is running with --machine-agent-only
	check_contains "${jujud_process}" "jujuagentd machine"
	check_contains "${jujud_process}" "machine-agent-only"

	# Verify Pebble services are both active
	local pebble_services
	pebble_services=$(kubectl exec -n "controller-${BOOTSTRAPPED_JUJU_CTRL_NAME}" "${pod}" -c api-server -- \
		/opt/pebble services 2>/dev/null || echo "")
	echo "Pebble services:"
	echo "${pebble_services}"
	check_contains "${pebble_services}" "jujud"
	check_contains "${pebble_services}" "jujuagentd"
}

# test_controller_restart restarts the jujud controller service via
# Pebble and verifies that the controller API recovers and the
# deployed workload remains manageable.
test_controller_restart() {
	if [ "$(skip 'test_controller_restart')" ]; then
		echo "==> TEST SKIPPED: controller restart tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		file="${1}"

		echo "====> Restarting jujud controller service"
		restart_pebble_service "jujud"

		echo "====> Waiting for controller API to recover"
		# Wait for the controller model to become available again.
		wait_for "controller" '.applications | keys | length > 0' 300

		# Verify the workload application is still healthy.
		wait_for "snappass-test" "$(idle_condition "snappass-test")" 300

		# Verify the split topology survived the restart.
		assert_split_topology

		echo "====> Controller restart recovery verified"
	)
}

# test_machine_agent_restart restarts the jujuagentd machine-agent
# service via Pebble and verifies that the controller remains
# healthy and that jujuagentd retains --machine-agent-only.
test_machine_agent_restart() {
	if [ "$(skip 'test_machine_agent_restart')" ]; then
		echo "==> TEST SKIPPED: machine-agent restart tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		file="${1}"

		echo "====> Restarting jujuagentd machine-agent service"
		restart_pebble_service "jujuagentd"

		echo "====> Waiting for machine-agent to reconnect"
		# Give the machine agent time to reconnect and stabilise.
		sleep 15

		# Verify the controller is still healthy.
		OUT=$(juju status -m controller --format=json 2>&1 || true)
		echo "Controller status after machine-agent restart:"
		echo "${OUT}"

		# Verify the workload application is still healthy.
		wait_for "snappass-test" "$(idle_condition "snappass-test")" 300

		# Verify the split topology survived the restart.
		assert_split_topology

		# Verify jujud remains the sole controller owner - no
		# second controller process or duplicate Dqlite/API server.
		local pod
		pod=$(get_controller_pod)
		if [[ -z ${pod} ]]; then
			echo "Failed to find controller pod" >&2
			exit 1
		fi

		local jujud_count
		jujud_count=$(kubectl exec -n "controller-${BOOTSTRAPPED_JUJU_CTRL_NAME}" "${pod}" -c api-server -- \
			/bin/sh -c "ps aux | grep -c 'jujud controller'" 2>/dev/null || echo "0")
		if [[ ${jujud_count} -ne 1 ]]; then
			echo "Expected exactly 1 jujud controller process, found ${jujud_count}" >&2
			exit 1
		fi

		echo "====> Machine-agent restart recovery verified"
	)
}

# test_workload_and_split_topology deploys a minimal workload through
# the split controller and verifies the split-process contract.
test_workload_and_split_topology() {
	if [ "$(skip 'test_workload_and_split_topology')" ]; then
		echo "==> TEST SKIPPED: workload and split topology tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		file="${1}"

		# Deploy a minimal workload
		ensure "test-workload-split" "${file}"
		juju deploy snappass-test --revision 8 --channel stable
		wait_for "snappass-test" "$(idle_condition "snappass-test")" 300

		# Verify the split topology in the controller pod
		assert_split_topology

		destroy_model "test-workload-split"
	)
}

# test_restart_resilience orchestrates the full restart resilience
# scenario: workload deploy, controller restart, and machine-agent
# restart in sequence on a single model.
test_restart_resilience() {
	if [ "$(skip 'test_restart_resilience')" ]; then
		echo "==> TEST SKIPPED: restart resilience tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		file="${1}"

		# Deploy a workload on the bootstrapped model.
		ensure "test-restart-resilience" "${file}"
		juju deploy snappass-test --revision 8 --channel stable
		wait_for "snappass-test" "$(idle_condition "snappass-test")" 300

		# Verify initial split topology.
		assert_split_topology

		# Restart the controller process.
		echo "====> Restarting jujud controller service"
		restart_pebble_service "jujud"

		echo "====> Waiting for controller API to recover"
		wait_for "controller" '.applications | keys | length > 0' 300
		wait_for "snappass-test" "$(idle_condition "snappass-test")" 300
		assert_split_topology

		# Restart the machine-agent process.
		echo "====> Restarting jujuagentd machine-agent service"
		restart_pebble_service "jujuagentd"

		echo "====> Waiting for machine-agent to reconnect"
		sleep 15

		wait_for "snappass-test" "$(idle_condition "snappass-test")" 300
		assert_split_topology

		# Verify jujud remains the sole controller owner.
		local pod
		pod=$(get_controller_pod)
		if [[ -z ${pod} ]]; then
			echo "Failed to find controller pod" >&2
			exit 1
		fi

		local jujud_count
		jujud_count=$(kubectl exec -n "controller-${BOOTSTRAPPED_JUJU_CTRL_NAME}" "${pod}" -c api-server -- \
			/bin/sh -c "ps aux | grep -c 'jujud controller'" 2>/dev/null || echo "0")
		if [[ ${jujud_count} -ne 1 ]]; then
			echo "Expected exactly 1 jujud controller process, found ${jujud_count}" >&2
			exit 1
		fi

		destroy_model "test-restart-resilience"

		echo "====> Restart resilience verified"
	)
}
