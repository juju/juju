# Query tracing and the Dqlite busy timeout are controller-owned
# settings: the machine agent's configuration never carries them. When
# one of them changes in the controller config, the jujud Dqlite-config
# updater writes the new value into the controller's local runtime.conf
# and, only after that write succeeds, restarts the controller so the
# new value takes effect. The tests below prove that delivery:
# machine-0's agent.conf has no tracing or Dqlite keys, the snap
# controller's runtime.conf carries the value, and no restart loop is
# observed within the settle window after the change. The exact restart
# count is covered by the dqliteconfigupdater unit tests.

# machine0_agent_conf_keys prints machine-0's agent.conf top-level key
# names, one per line, with the values stripped. The file holds the
# machine credential, so the values never leave the controller machine.
machine0_agent_conf_keys() {
	juju exec -m controller --machine 0 \
		"sudo grep -oE '^[A-Za-z][A-Za-z0-9_-]*:' /var/lib/juju/agents/machine-0/agent.conf"
}

# machine0_agent_conf_line prints the top-level YAML line for a
# non-secret key such as tag. Never call it for a credential-bearing key.
machine0_agent_conf_line() {
	juju exec -m controller --machine 0 \
		"sudo grep -E '^${1}:' /var/lib/juju/agents/machine-0/agent.conf"
}

# assert_machine0_agent_conf_has_no_dqlite_keys proves the machine agent's
# configuration carries none of the controller-owned Dqlite keys. The tag
# read anchors the key list so a failed read cannot make the negative
# assertions pass vacuously.
assert_machine0_agent_conf_has_no_dqlite_keys() {
	local keys tag_line key
	keys=$(machine0_agent_conf_keys)
	tag_line=$(machine0_agent_conf_line "tag")
	if [[ ${tag_line} != "tag: machine-0" ]]; then
		echo "Failed: could not read machine-0 agent.conf (tag \"${tag_line}\")." >&2
		return 1
	fi
	for key in querytracingenabled querytracingthreshold dqlitebusytimeout dqlite-port; do
		if printf "%s\n" "${keys}" | grep -qx "${key}:"; then
			echo "Failed: machine-0 agent.conf must not contain key \"${key}\"." >&2
			return 1
		fi
	done
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

# require_controller_jujud_pid prints the running controller PID, or fails
# rather than allowing a missing baseline PID to look like a later restart.
require_controller_jujud_pid() {
	local pid
	pid=$(controller_jujud_pid)
	if [[ -z ${pid} ]]; then
		echo "Failed: no jujud controller process found before the controller-config change." >&2
		return 1
	fi
	printf '%s\n' "${pid}"
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

	# No restart loop: once the delivered values match the controller
	# config, the updater's reconciliation is the baseline and should
	# not restart again. This only observes a 30-second window; the exact
	# restart count is asserted by the dqliteconfigupdater unit tests.
	sleep 30
	if [[ "$(controller_jujud_pid)" != "${pid_after}" ]]; then
		echo "Failed: controller restarted again within the 30-second settle window after the config change." >&2
		return 1
	fi
}

run_query_tracing_enabled() {
	echo

	file="${TEST_DIR}/query_tracing_enabled.log"

	ensure "query-tracing-enabled" "${file}"

	# The Dqlite settings are no longer part of the machine agent's
	# configuration contract.
	assert_machine0_agent_conf_has_no_dqlite_keys

	check_contains "$(controller_runtime_conf_key "query-tracing-enabled")" "query-tracing-enabled: false"

	if ! pid_before=$(require_controller_jujud_pid); then
		return 1
	fi

	juju controller-config query-tracing-enabled=true

	wait_controller_restart_with_value "${pid_before}" "query-tracing-enabled" "query-tracing-enabled: true"

	# Set the value back in case we are reusing a controller
	if ! pid_before=$(require_controller_jujud_pid); then
		return 1
	fi
	juju controller-config query-tracing-enabled=false
	wait_controller_restart_with_value "${pid_before}" "query-tracing-enabled" "query-tracing-enabled: false"

	destroy_model "query-tracing-enabled"
}

run_query_tracing_threshold() {
	echo

	file="${TEST_DIR}/query_tracing_threshold.log"

	ensure "query-tracing-threshold" "${file}"

	# The Dqlite settings are no longer part of the machine agent's
	# configuration contract.
	assert_machine0_agent_conf_has_no_dqlite_keys

	check_contains "$(controller_runtime_conf_key "query-tracing-threshold")" "1s"

	if ! pid_before=$(require_controller_jujud_pid); then
		return 1
	fi

	juju controller-config query-tracing-threshold="42ms"

	wait_controller_restart_with_value "${pid_before}" "query-tracing-threshold" "query-tracing-threshold: 42ms"

	# Set the value back in case we are reusing a controller
	if ! pid_before=$(require_controller_jujud_pid); then
		return 1
	fi
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
