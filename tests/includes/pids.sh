DAEMON_SCOPE_DEPTH=0

_daemon_scope_label() {
	local depth=${1:-${DAEMON_SCOPE_DEPTH}}
	local scope_label
	scope_label="scope_index"

	local i
	for ((i = 0; i < depth; i++)); do
		scope_label="${scope_label}_${i}"
	done
	echo "$scope_label"
}

# track_daemon_pid appends the provided PID to the
track_daemon_pid() {
	local pid
	pid=$1
	echo "${pid} $(_daemon_scope_label)" >>"${TEST_DIR}/cleanup"
}

# push_daemon_scope creates a new daemon tracking scope for nested run calls
push_daemon_scope() {
	DAEMON_SCOPE_DEPTH=$((DAEMON_SCOPE_DEPTH + 1))
}

# pop_daemon_scope removes and cleans up the current daemon scope
pop_daemon_scope() {
	local expected_depth
	expected_depth=$1

	# Kill all daemons whose scope matches current_scope exactly or is a child
	# of it.
	if [[ -f "${TEST_DIR}/cleanup" ]]; then
		local pid
		local current_scope
		current_scope="$(_daemon_scope_label $expected_depth)"
		while IFS= read -r pid; do
			if kill -0 "${pid}" 2>/dev/null; then
				kill -9 "${pid}" >/dev/null 2>&1 || true
				echo "==> Killed daemon (PID is $(green "${pid}"))"
			fi
		done < <(awk -v scope="${current_scope}" '
			$2 == scope || index($2, scope "_") == 1 { print $1 }
		' "${TEST_DIR}/cleanup")
	fi

	if [[ ${DAEMON_SCOPE_DEPTH} -eq 0 ]]; then
		return
	fi
	if [[ ${DAEMON_SCOPE_DEPTH} -ne ${expected_depth} ]]; then
		return
	fi
	DAEMON_SCOPE_DEPTH=$((DAEMON_SCOPE_DEPTH - 1))
}

# daemon runs a command in the background and tracks its PID for cleanup
daemon() {
	if [[ ${DAEMON_SCOPE_DEPTH} -eq 0 ]]; then
		echo "ERROR: daemon() called outside of run() scope" >&2
		return 1
	fi

	local pid
	local program_name
	program_name=$(basename "$1")
	(
		exec >"${TEST_DIR}/${TEST_CURRENT}-${program_name}-${BASHPID}.log" 2>&1
		exec "$@"
	) &
	pid=$!

	# Append PID and current scope label to the cleanup file
	track_daemon_pid "$pid"

	echo "==> Started daemon (PID is $(green "${pid}"))"
}
