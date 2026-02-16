# Stack to track daemon PIDs for nested run scopes
# Each element is a space-separated list of PIDs for that scope level
DAEMON_SCOPE_STACK=()
DAEMON_SCOPE_DEPTH=0

# push_daemon_scope creates a new daemon tracking scope for nested run calls
push_daemon_scope() {
	DAEMON_SCOPE_STACK+=("")
	DAEMON_SCOPE_DEPTH=$((DAEMON_SCOPE_DEPTH + 1))
}

# pop_daemon_scope removes and cleans up the current daemon scope
pop_daemon_scope() {
	local expected_depth=$1
	if [[ ${DAEMON_SCOPE_DEPTH} -ne ${expected_depth} ]]; then
		return
	fi
	
	if [[ ${DAEMON_SCOPE_DEPTH} -eq 0 ]]; then
		return
	fi
	
	local scope_index=$((DAEMON_SCOPE_DEPTH - 1))
	local pids="${DAEMON_SCOPE_STACK[${scope_index}]}"
	
	# Kill all daemons in this scope
	local pid
	for pid in ${pids}; do
		if kill -0 "${pid}" 2>/dev/null; then
			kill -9 "${pid}" >/dev/null 2>&1 || true
			echo "==> Killed daemon (PID is $(green "${pid}"))"
		fi
	done
	
	# Remove this scope from the stack
	unset 'DAEMON_SCOPE_STACK[${scope_index}]'
	DAEMON_SCOPE_DEPTH=$((DAEMON_SCOPE_DEPTH - 1))
}

# daemon runs a command in the background and tracks its PID for cleanup
daemon() {
	if [[ ${DAEMON_SCOPE_DEPTH} -eq 0 ]]; then
		echo "ERROR: daemon() called outside of run() scope" >&2
		return 1
	fi
	
	local program_name=$(basename "$1")
	(
		exec >"${TEST_DIR}/${TEST_CURRENT}-${program_name}-${BASHPID}.log" 2>&1
		exec "$@"
	) &
	local pid=$!
	
	# Add PID to the current scope
	local scope_index=$((DAEMON_SCOPE_DEPTH - 1))
	if [[ -z ${DAEMON_SCOPE_STACK[${scope_index}]} ]]; then
		DAEMON_SCOPE_STACK[${scope_index}]="${pid}"
	else
		DAEMON_SCOPE_STACK[${scope_index}]="${DAEMON_SCOPE_STACK[${scope_index}]} ${pid}"
	fi
	
	echo "==> Started daemon (PID is $(green "${pid}"))"
}

# run a command and immediately terminate the script when any error occurs.
run() {
	CMD="${1}"

	if [[ -n ${RUN_SUBTEST} ]]; then
		# shellcheck disable=SC2143
		if [[ ! "$(echo "${RUN_SUBTEST}" | grep -E "^${CMD}$")" ]]; then
			echo "SKIPPING: ${RUN_SUBTEST} ${CMD}"
			return 0
		fi
	fi

	DESC=$(echo "${1}" | sed -E "s/^run_//g" | sed -E "s/_/ /g")

	echo "===> [   ] Running: ${DESC}"

	START_TIME=$(date +%s)
	
	push_daemon_scope
	local expected_scope_depth=${DAEMON_SCOPE_DEPTH}
	trap "pop_daemon_scope ${expected_scope_depth}" EXIT

	set_verbosity

	local pid
	if [[ ${VERBOSE} -gt 1 ]]; then
		touch "${TEST_DIR}/${TEST_CURRENT}.log"
		tail -f "${TEST_DIR}/${TEST_CURRENT}.log" 2>/dev/null &
		pid=$!

		# SIGKILL it with fire, as we don't know what state we're in.
		trap "kill -9 ${pid} >/dev/null 2>&1 || true; pop_daemon_scope ${expected_scope_depth}" EXIT
	fi

	"${CMD}" "$@" >"${TEST_DIR}/${TEST_CURRENT}.log" 2>&1
	if [[ ${VERBOSE} -gt 1 ]]; then
		# SIGKILL because it should be safe to do so.
		kill -9 "${pid}" >/dev/null 2>&1 || true
	fi
	pop_daemon_scope ${expected_scope_depth}

	END_TIME=$(date +%s)

	echo -e "\r\033[1A\033[0K===> [ $(green "✔") ] Success: ${DESC} ($((END_TIME - START_TIME))s)"
}

# run_linter will run until the end of a pipeline even if there is a failure.
# This is different from `run` as we require the output of a linter.
run_linter() {
	CMD="${1}"

	if [[ -n ${RUN_SUBTEST} ]]; then
		# shellcheck disable=SC2143
		if [[ ! "$(echo "${RUN_SUBTEST}" | grep -E "^${CMD}$")" ]]; then
			echo "SKIPPING: ${RUN_SUBTEST} ${CMD}"
			exit 0
		fi
	fi

	DESC=$(echo "${1}" | sed -E "s/^run_//g" | sed -E "s/_/ /g")

	echo "===> [   ] Running: ${DESC}"

	START_TIME=$(date +%s)

	# Prevent the sub-shell from killing our script if that sub-shell fails on an
	# error. We need this so that we can capture the full output and collect the
	# exit code when it does fail.
	# Do not remove or none of the tests will report correctly!
	set +e
	set -o pipefail

	cmd_output=$("${CMD}" "$@" 2>&1)
	cmd_status=$?

	set_verbosity
	set +o pipefail

	# Only output if it's not empty.
	if [[ -n ${cmd_output} ]]; then
		echo -e "${cmd_output}" | OUTPUT "${TEST_DIR}/${TEST_CURRENT}.log"
	fi

	END_TIME=$(date +%s)

	if [[ ${cmd_status} -eq 0 ]]; then
		echo -e "\r\033[1A\033[0K===> [ $(green "✔") ] Success: ${DESC} ($((END_TIME - START_TIME))s)"
	else
		echo -e "\r\033[1A\033[0K===> [ $(red "x") ] Fail: ${DESC} ($((END_TIME - START_TIME))s)"
		exit 1
	fi
}

skip() {
	CMD="${1}"

	if [[ -n ${RUN_LIST} ]]; then
		# shellcheck disable=SC2143,SC2046
		if [[ ! $(echo "${RUN_LIST}" | grep -w "${CMD}") ]]; then
			echo "SKIP"
			exit 1
		fi
	fi

	# shellcheck disable=SC2143,SC2046
	if [[ $(echo "${SKIP_LIST:-}" | grep -w "${CMD}") ]]; then
		echo "SKIP"
		exit 1
	fi
	return
}
