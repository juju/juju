# Helpers for CAAS controller-process and machine-agent restart resilience.
# These functions interact with the Pebble service manager in the
# Kubernetes controller pod to restart individual services and verify
# the split-process topology after each recovery event.

get_controller_pod() {
	local pod
	# Controller pods are labelled with the Juju controller stack name
	# constant, not the bootstrap controller name; only the namespace is
	# derived from the controller name.
	pod=$(kubectl get pods -n "controller-${BOOTSTRAPPED_JUJU_CTRL_NAME}" \
		-l "app.kubernetes.io/name=controller" \
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
# and jujuagentd must run as the machine agent.
assert_split_topology() {
	local pod
	pod=$(get_controller_pod)
	if [[ -z ${pod} ]]; then
		return 1
	fi

	echo "Verifying split controller topology in pod ${pod}"

	# Capture the full process list once; it contains both the jujud
	# controller process and the jujuagentd machine-agent process.
	local process_list
	process_list=$(kubectl exec -n "controller-${BOOTSTRAPPED_JUJU_CTRL_NAME}" "${pod}" -c api-server -- \
		/bin/sh -c "ps aux" 2>/dev/null || echo "")
	echo "Process list from controller pod:"
	echo "${process_list}"
	check_contains "${process_list}" "jujud controller"
	check_contains "${process_list}" "jujuagentd machine"

	# Verify Pebble services are both active
	local pebble_services
	pebble_services=$(kubectl exec -n "controller-${BOOTSTRAPPED_JUJU_CTRL_NAME}" "${pod}" -c api-server -- \
		/opt/pebble services 2>/dev/null || echo "")
	echo "Pebble services:"
	echo "${pebble_services}"
	check_contains "${pebble_services}" "jujud"
	check_contains "${pebble_services}" "jujuagentd"
}

# pod_exec_sh runs a shell command in the controller pod's api-server
# container. The pod's processes run as the juju user, so agent-owned
# files under /var/lib/juju are readable without sudo.
pod_exec_sh() {
	local pod cmd
	pod=$(get_controller_pod)
	if [[ -z ${pod} ]]; then
		return 1
	fi
	cmd=${1}
	kubectl exec -n "controller-${BOOTSTRAPPED_JUJU_CTRL_NAME}" "${pod}" -c api-server -- \
		/bin/sh -c "${cmd}" 2>/dev/null || echo ""
}

# conf_has_key reports whether the key-name list contains the given key.
conf_has_key() {
	printf "%s\n" "${2}" | grep -qx "${1}:"
}

# machine_agent_conf_keys prints the controller pod machine agent's
# top-level agent.conf key names, one per line, with the values
# stripped. The file holds the machine credential, so the values never
# leave the pod.
machine_agent_conf_keys() {
	pod_exec_sh "grep -oE '^[A-Za-z][A-Za-z0-9_-]*:' /var/lib/juju/agents/machine-0/agent.conf"
}

# machine_agent_conf_line prints the top-level YAML line for a
# non-secret key such as tag or apiaddresses. Never call it for a
# credential-bearing key.
machine_agent_conf_line() {
	pod_exec_sh "grep -E '^${1}:' /var/lib/juju/agents/machine-0/agent.conf"
}

# assert_machine_agent_conf_trimmed asserts the controller pod machine
# agent's configuration carries the controller-0 identity and
# connectivity state only: no controller-serving or Dqlite field is
# serialized.
assert_machine_agent_conf_trimmed() {
	local keys tag_line addresses_line key
	keys=$(machine_agent_conf_keys)

	tag_line=$(machine_agent_conf_line "tag")
	if [[ ${tag_line} != "tag: controller-0" ]]; then
		echo "Unexpected pod machine agent tag \"${tag_line}\", expected controller-0" >&2
		return 1
	fi

	# Required connectivity state: addresses, credential, trust anchor.
	addresses_line=$(machine_agent_conf_line "apiaddresses")
	if [[ -z ${addresses_line} || ${addresses_line} == "apiaddresses: []" ]]; then
		echo "Pod machine agent config has no API addresses" >&2
		return 1
	fi
	# The pod machine agent connects with OnlyConnect and never rotates,
	# so the pre-rendered controller-node credential stays in
	# oldpassword. A rotation-capable agent would write apipassword, so
	# accept either key rather than assuming rotation has happened.
	if ! conf_has_key "apipassword" "${keys}" && ! conf_has_key "oldpassword" "${keys}"; then
		echo "Pod machine agent config has no API credential" >&2
		return 1
	fi
	if ! conf_has_key "cacert" "${keys}"; then
		echo "Pod machine agent config has no CA certificate" >&2
		return 1
	fi

	# No controller serving information on the machine-agent path.
	for key in controllercert controllerkey caprivatekey apiport systemidentity statepassword stateaddresses; do
		if conf_has_key "${key}" "${keys}"; then
			echo "Pod machine agent config must not contain key \"${key}\"" >&2
			return 1
		fi
	done

	# Dqlite settings are delivered via runtime.conf, not agent.conf.
	for key in querytracingenabled querytracingthreshold dqlitebusytimeout dqlite-port; do
		if conf_has_key "${key}" "${keys}"; then
			echo "Pod machine agent config must not contain key \"${key}\"" >&2
			return 1
		fi
	done

	echo "Pod machine agent config carries connectivity state only"
}

# machine_agent_credential_digest prints a SHA-256 digest of the machine
# agent's API credential lines (apipassword and/or oldpassword) so the
# credential can be compared across restarts without being logged. The
# digest is computed inside the pod, so the credential never leaves it.
machine_agent_credential_digest() {
	pod_exec_sh "if grep -qE '^(apipassword|oldpassword):' /var/lib/juju/agents/machine-0/agent.conf; then grep -E '^(apipassword|oldpassword):' /var/lib/juju/agents/machine-0/agent.conf | sha256sum | awk '{print \$1}'; fi"
}

# jujud_pid prints the PID of the jujud controller process in the
# controller pod's api-server container, used to observe controller
# restarts.
jujud_pid() {
	pod_exec_sh "ps aux" | awk '/[j]ujud controller/ {print $2}' | head -n 1
}

# runtime_conf_key_line prints the top-level YAML line for the given key
# from the controller pod's runtime.conf. The file holds controller
# credentials, so only the requested line leaves the pod.
runtime_conf_key_line() {
	pod_exec_sh "grep -E '^${1}:' /var/lib/juju/controller/runtime.conf"
}

# set_dqlite_config_value sets the given controller-config key to value
# and waits for the controller to restart with the value delivered
# through runtime.conf.
set_dqlite_config_value() {
	local key value pid_before pid_after key_line attempt
	key=${1}
	value=${2}

	pid_before=$(jujud_pid)
	if [[ -z ${pid_before} ]]; then
		echo "No jujud controller process found before ${key} update" >&2
		return 1
	fi
	echo "jujud controller pid before ${key}=${value}: ${pid_before}"

	juju controller-config "${key}=${value}"

	# The updater writes runtime.conf, then exits; Pebble restarts the
	# jujud service. Wait for the process identity to change.
	attempt=0
	pid_after=""
	until [[ -n ${pid_after} && ${pid_after} != "${pid_before}" ]]; do
		pid_after=$(jujud_pid)
		if [[ -n ${pid_after} && ${pid_after} != "${pid_before}" ]]; then
			break
		fi
		echo "[+] (attempt ${attempt}) waiting for controller restart"
		if [[ ${attempt} -ge 60 ]]; then
			echo "Failed: controller did not restart after ${key}=${value}" >&2
			return 1
		fi
		sleep 5
		attempt=$((attempt + 1))
	done
	echo "jujud controller pid after ${key}=${value}: ${pid_after}"

	# The restart applied the value through the runtime.conf delivery
	# cache. The write happens before the restart, so it must already be
	# visible.
	key_line=$(runtime_conf_key_line "${key}")
	if [[ ${key_line} != "${key}: ${value}" ]]; then
		echo "runtime.conf does not carry ${key}: ${value} (got \"${key_line}\")" >&2
		return 1
	fi
}

# assert_dqlite_config_update proves the Dqlite controller-config update
# path end to end: changing a mutable Dqlite controller-config value
# makes the jujud Dqlite-config updater write the new value into the
# controller's local runtime.conf and restart the controller. It then
# asserts that no further restart is observed within a 30-second settle
# window; the exact restart count is covered by the dqliteconfigupdater
# unit tests. The value is restored to its default at the end so a
# reused controller starts from a known state.
assert_dqlite_config_update() {
	local key_line pid_after pid_settled

	# The test asserts the false -> true transition, so the controller
	# must start from the known default.
	key_line=$(runtime_conf_key_line "query-tracing-enabled")
	if [[ ${key_line} != "query-tracing-enabled: false" ]]; then
		echo "Controller is not in a known state: runtime.conf has \"${key_line}\", expected \"query-tracing-enabled: false\"" >&2
		return 1
	fi

	set_dqlite_config_value "query-tracing-enabled" "true"
	pid_after=$(jujud_pid)

	# The controller must recover and keep serving.
	wait_for "snappass-test" "$(idle_condition "snappass-test")" 300
	assert_split_topology

	# No restart loop: after the reconciliation the updated values match,
	# so the controller should not restart again. This only observes a
	# 30-second window; the exact restart count is asserted by the
	# dqliteconfigupdater unit tests.
	sleep 30
	pid_settled=$(jujud_pid)
	if [[ ${pid_settled} != "${pid_after}" ]]; then
		echo "Controller restarted again within the 30-second settle window: pid ${pid_after} became ${pid_settled}" >&2
		return 1
	fi
	echo "No controller restart observed within the 30-second settle window"

	# Restore the default so a reused controller starts from a known
	# state. This restart is expected and is separate from the
	# no-restart-loop assertion above.
	set_dqlite_config_value "query-tracing-enabled" "false"
	wait_for "snappass-test" "$(idle_condition "snappass-test")" 300
	echo "Dqlite config restored to query-tracing-enabled=false"
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
		# wait_for greps the query output for the application name, so
		# waiting for the application to be reported again confirms the
		# controller API is answering.
		wait_for "snappass-test" '.applications | keys | .[]' 300

		# Verify the workload application is still healthy.
		wait_for "snappass-test" "$(idle_condition "snappass-test")" 300

		# Verify the split topology survived the restart.
		assert_split_topology

		echo "====> Controller restart recovery verified"
	)
}

# test_machine_agent_restart restarts the jujuagentd machine-agent
# service via Pebble and verifies that the controller remains healthy
# and that jujuagentd continues running as the machine agent.
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
			/bin/sh -c "ps aux | grep -c '[j]ujud controller'" 2>/dev/null || echo "0")
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
# scenario on a single model: workload deploy, controller restart, and
# machine-agent restart, with the machine-agent configuration
# assertions (the controller pod machine agent keeps its controller-0
# identity, its configuration carries connectivity state only, and its
# credential is never rotated) and the Dqlite controller-config update
# delivery proof (the value applied with no restart loop observed in
# the settle window).
test_restart_resilience() {
	if [ "$(skip 'test_restart_resilience')" ]; then
		echo "==> TEST SKIPPED: restart resilience tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		local digest_before digest_after
		file="${1}"

		# Deploy a workload on the bootstrapped model.
		ensure "test-restart-resilience" "${file}"
		juju deploy snappass-test --revision 8 --channel stable
		wait_for "snappass-test" "$(idle_condition "snappass-test")" 300

		# Verify initial split topology.
		assert_split_topology

		# Capture the machine-agent configuration boundary and a
		# non-logged credential digest before any restart.
		assert_machine_agent_conf_trimmed
		digest_before=$(machine_agent_credential_digest)
		if [[ -z ${digest_before} ]]; then
			echo "Failed to digest the pod machine agent credential" >&2
			exit 1
		fi

		# Restart the controller process.
		echo "====> Restarting jujud controller service"
		restart_pebble_service "jujud"

		echo "====> Waiting for controller API to recover"
		# wait_for greps the query output for the application name, so
		# waiting for the application to be reported again confirms the
		# controller API is answering.
		wait_for "snappass-test" '.applications | keys | .[]' 300
		wait_for "snappass-test" "$(idle_condition "snappass-test")" 300
		assert_split_topology

		# Restart the machine-agent process.
		echo "====> Restarting jujuagentd machine-agent service"
		restart_pebble_service "jujuagentd"

		echo "====> Waiting for machine-agent to reconnect"
		sleep 15

		wait_for "snappass-test" "$(idle_condition "snappass-test")" 300
		assert_split_topology

		# The machine agent connects with OnlyConnect and never rotates
		# its controller-node credential: the digest must be unchanged
		# across the restart, and the configuration boundary must hold.
		digest_after=$(machine_agent_credential_digest)
		if [[ -z ${digest_after} ]]; then
			echo "Failed to digest the pod machine agent credential after restart" >&2
			exit 1
		fi
		if [[ ${digest_after} != "${digest_before}" ]]; then
			echo "Pod machine agent credential changed across the jujuagentd restart (rotation is forbidden)" >&2
			exit 1
		fi
		echo "====> Machine-agent credential stable across restart"
		assert_machine_agent_conf_trimmed

		# Verify jujud remains the sole controller owner.
		local pod
		pod=$(get_controller_pod)
		if [[ -z ${pod} ]]; then
			echo "Failed to find controller pod" >&2
			exit 1
		fi

		local jujud_count
		jujud_count=$(kubectl exec -n "controller-${BOOTSTRAPPED_JUJU_CTRL_NAME}" "${pod}" -c api-server -- \
			/bin/sh -c "ps aux | grep -c '[j]ujud controller'" 2>/dev/null || echo "0")
		if [[ ${jujud_count} -ne 1 ]]; then
			echo "Expected exactly 1 jujud controller process, found ${jujud_count}" >&2
			exit 1
		fi

		# Prove the Dqlite controller-config update delivery: one
		# controller restart with the value applied.
		assert_dqlite_config_update

		destroy_model "test-restart-resilience"

		echo "====> Restart resilience verified"
	)
}
