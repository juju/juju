# Query tracing and the Dqlite busy timeout are controller-owned
# settings: the machine agent's configuration never carries them. When
# one of them changes in the controller config, the jujud Dqlite-config
# updater writes the new value into the controller's local runtime.conf
# and, only after that write succeeds, restarts the controller exactly
# once so the new value takes effect. The tests below prove that
# delivery: machine-0's agent.conf has no tracing or Dqlite keys, the
# snap controller's runtime.conf carries the value, and the controller
# restarts once per change.

# machine0_agent_conf_key greps machine-0's agent.conf for the given
# pattern. The file holds the machine credential, so callers only probe
# for key names and never echo the content.
machine0_agent_conf_key() {
	# shellcheck disable=SC2046
	# Not using juju_exec_output: sudo cat produces no stderr, so the
	# stdout/stderr mixing bug does not apply.
	echo $(juju exec -m controller --machine 0 'sudo cat /var/lib/juju/agents/machine-0/agent.conf' | grep "$1")
}

# controller_runtime_conf_key greps the snap controller's runtime.conf
# for the given top-level key.
controller_runtime_conf_key() {
	# shellcheck disable=SC2046
	# Not using juju_exec_output: sudo cat produces no stderr, so the
	# stdout/stderr mixing bug does not apply.
	echo $(juju exec -m controller --machine 0 'sudo cat /var/snap/jujud/current/runtime.conf' | grep -E "^$1:")
}

# controller_jujud_pid prints the PID of the jujud controller process on
# machine-0, used to observe controller restarts.
controller_jujud_pid() {
	# shellcheck disable=SC2046
	echo $(juju exec -m controller --machine 0 'ps aux' | awk '/[j]ujud controller/ {print $2}' | head -n 1)
}

# wait_controller_restart_with_value waits until the jujud controller
# process has restarted (PID change from ${pid_before}) and runtime.conf
# carries the expected top-level key line. The updater writes
# runtime.conf before the controller exits, so a restarted controller
# implies the value is already delivered.
wait_controller_restart_with_value() {
	local pid_before key expected key_line attempt pid_after
	pid_before=${1}
	key=${2}
	expected=${3}

	attempt=0
	pid_after=""
	until [[ -n ${pid_after} && ${pid_after} != "${pid_before}" ]]; do
		pid_after=$(controller_jujud_pid)
		if [[ -n ${pid_after} && ${pid_after} != "${pid_before}" ]]; then
			break
		fi
		echo "[+] (attempt ${attempt}) waiting for controller restart"
		# This will attempt to wait for 3 minutes before failing out.
		if [[ ${attempt} -ge 36 ]]; then
			echo "Failed: expected the controller to restart after the controller-config change." >&2
			return 1
		fi
		sleep 5
		attempt=$((attempt + 1))
	done

	key_line=$(controller_runtime_conf_key "${key}")
	if [[ ${key_line} != "${expected}" ]]; then
		echo "Failed: runtime.conf ${key} is \"${key_line}\", expected \"${expected}\"." >&2
		return 1
	fi

	# Exactly one restart: once the delivered values match the
	# controller config, the updater's reconciliation is the baseline
	# and must not restart again.
	sleep 30
	if [[ "$(controller_jujud_pid)" != "${pid_after}" ]]; then
		echo "Failed: controller restarted more than once for the config change." >&2
		return 1
	fi
}

run_query_tracing_enabled() {
	echo

	file="${TEST_DIR}/query_tracing_enabled.log"

	ensure "query-tracing-enabled" "${file}"

	# The Dqlite setting is no longer part of the machine agent's
	# configuration contract.
	check_not_contains "$(machine0_agent_conf_key "querytracingenabled")" querytracingenabled

	check_contains "$(controller_runtime_conf_key "query-tracing-enabled")" "query-tracing-enabled: false"

	pid_before=$(controller_jujud_pid)

	juju controller-config query-tracing-enabled=true

	wait_controller_restart_with_value "${pid_before}" "query-tracing-enabled" "query-tracing-enabled: true"

	# Set the value back in case we are reusing a controller
	pid_before=$(controller_jujud_pid)
	juju controller-config query-tracing-enabled=false
	wait_controller_restart_with_value "${pid_before}" "query-tracing-enabled" "query-tracing-enabled: false"

	destroy_model "query-tracing-enabled"
}

run_query_tracing_threshold() {
	echo

	file="${TEST_DIR}/query_tracing_threshold.log"

	ensure "query-tracing-threshold" "${file}"

	# The Dqlite setting is no longer part of the machine agent's
	# configuration contract.
	check_not_contains "$(machine0_agent_conf_key "querytracingthreshold")" querytracingthreshold

	check_contains "$(controller_runtime_conf_key "query-tracing-threshold")" "1s"

	pid_before=$(controller_jujud_pid)

	juju controller-config query-tracing-threshold="42ms"

	wait_controller_restart_with_value "${pid_before}" "query-tracing-threshold" "query-tracing-threshold: 42ms"

	# Set the value back in case we are reusing a controller
	pid_before=$(controller_jujud_pid)
	juju controller-config query-tracing-threshold="1s"
	wait_controller_restart_with_value "${pid_before}" "query-tracing-threshold" "query-tracing-threshold: 1s"

	destroy_model "query-tracing-threshold"
}

test_query_tracing() {
	if [[ -n "$(skip 'test_query_tracing')" ]]; then
		echo "==> SKIP: Asked to skip controller query tracing tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_query_tracing_enabled"
		run "run_query_tracing_threshold"
	)
}
