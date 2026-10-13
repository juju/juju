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

# conf_key_line prints the top-level YAML line for the given key from a
# configuration body. Used to probe agent.conf and runtime.conf without
# ever echoing credential-bearing content.
conf_key_line() {
	printf "%s" "${2}" | grep -E "^${1}:" || true
}

# machine_agent_conf reads the controller pod machine agent's live
# agent.conf. The output must never be logged: it holds the machine
# credential. Only key-probing helpers and the digest helper consume it.
machine_agent_conf() {
	pod_exec_sh "cat /var/lib/juju/agents/machine-0/agent.conf"
}

# assert_machine_agent_conf_trimmed asserts the controller pod machine
# agent's configuration carries the controller-0 identity and
# connectivity state only: no controller-serving or Dqlite field is
# serialized.
assert_machine_agent_conf_trimmed() {
	local conf tag_line
	conf=$(machine_agent_conf)

	tag_line=$(conf_key_line "tag" "${conf}")
	if [[ ${tag_line} != "tag: controller-0" ]]; then
		echo "Unexpected pod machine agent tag \"${tag_line}\", expected controller-0" >&2
		return 1
	fi

	# Required connectivity state: addresses, credential, trust anchor.
	local addresses_line
	addresses_line=$(conf_key_line "apiaddresses" "${conf}")
	if [[ -z ${addresses_line} || ${addresses_line} == "apiaddresses: []" ]]; then
		echo "Pod machine agent config has no API addresses" >&2
		return 1
	fi
	if [[ -z $(conf_key_line "apipassword" "${conf}") ]]; then
		echo "Pod machine agent config has no API password" >&2
		return 1
	fi
	if [[ -z $(conf_key_line "cacert" "${conf}") ]]; then
		echo "Pod machine agent config has no CA certificate" >&2
		return 1
	fi

	# No controller serving information on the machine-agent path.
	local key
	for key in controllercert controllerkey caprivatekey apiport systemidentity statepassword stateaddresses; do
		if [[ -n $(conf_key_line "${key}" "${conf}") ]]; then
			echo "Pod machine agent config must not contain key \"${key}\"" >&2
			return 1
		fi
	done

	# Dqlite settings are delivered via runtime.conf, not agent.conf.
	for key in querytracingenabled querytracingthreshold dqlitebusytimeout dqlite-port; do
		if [[ -n $(conf_key_line "${key}" "${conf}") ]]; then
			echo "Pod machine agent config must not contain key \"${key}\"" >&2
			return 1
		fi
	done

	echo "Pod machine agent config carries connectivity state only"
}

# machine_agent_credential_digest prints a SHA-256 digest of the machine
# agent's API password so the credential can be compared across restarts
# without being logged.
machine_agent_credential_digest() {
	local conf
	conf=$(machine_agent_conf)
	if [[ -z $(conf_key_line "apipassword" "${conf}") ]]; then
		echo ""
		return 1
	fi
	printf "%s" "${conf}" | grep -E "^apipassword:" | sha256sum | awk '{print $1}'
}

# jujud_pid prints the PID of the jujud controller process in the
# controller pod's api-server container, used to observe controller
# restarts.
jujud_pid() {
	pod_exec_sh "ps aux" | awk '/[j]ujud controller/ {print $2}' | head -n 1
}

# runtime_conf_key_line prints the top-level YAML line for the given key
# from the controller pod's runtime.conf.
runtime_conf_key_line() {
	conf_key_line "${1}" "$(pod_exec_sh "cat /var/lib/juju/controller/runtime.conf")"
}

# assert_dqlite_config_update proves the Dqlite controller-config update
# path end to end: changing a mutable Dqlite controller-config value
# makes the jujud Dqlite-config updater write the new value into the
# controller's local runtime.conf and restart the controller exactly
# once.
assert_dqlite_config_update() {
	local pid_before pid_after pid_settled key_line attempt

	pid_before=$(jujud_pid)
	if [[ -z ${pid_before} ]]; then
		echo "No jujud controller process found before config update" >&2
		return 1
	fi
	echo "jujud controller pid before config update: ${pid_before}"

	juju controller-config query-tracing-enabled=true

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
			echo "Failed: controller did not restart after query-tracing-enabled=true" >&2
			return 1
		fi
		sleep 5
		attempt=$((attempt + 1))
	done
	echo "jujud controller pid after restart: ${pid_after}"

	# The restart applied the value through the runtime.conf delivery
	# cache. The write happens before the restart, so it must already be
	# visible.
	key_line=$(runtime_conf_key_line "query-tracing-enabled")
	if [[ ${key_line} != "query-tracing-enabled: true" ]]; then
		echo "runtime.conf does not carry query-tracing-enabled: true (got \"${key_line}\")" >&2
		return 1
	fi

	# The controller must recover and keep serving.
	wait_for "snappass-test" "$(idle_condition "snappass-test")" 300
	assert_split_topology

	# Exactly one restart: after the reconciliation the updated values
	# match, so a restart loop must not occur. Give the controller a
	# settle window and assert the process identity is stable.
	sleep 30
	pid_settled=$(jujud_pid)
	if [[ ${pid_settled} != "${pid_after}" ]]; then
		echo "Unexpected second controller restart: pid ${pid_after} became ${pid_settled}" >&2
		return 1
	fi
	echo "Dqlite config update caused exactly one controller restart"
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
# delivery proof (exactly one controller restart with the value
# applied).
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
