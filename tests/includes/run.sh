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
	local expected_scope_depth
	expected_scope_depth=${DAEMON_SCOPE_DEPTH}
	# shellcheck disable=SC2064
	trap "pop_daemon_scope ${expected_scope_depth}" RETURN

	set_verbosity

	local pid
	if [[ ${VERBOSE} -gt 1 ]]; then
		touch "${TEST_DIR}/${TEST_CURRENT}.log"
		tail -f "${TEST_DIR}/${TEST_CURRENT}.log" 2>/dev/null &
		pid=$!

		track_daemon_pid "$pid"
	fi

	"${CMD}" "$@" >"${TEST_DIR}/${TEST_CURRENT}.log" 2>&1
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
	# For each command, check if it would be skipped (absent from RUN_LIST when
	# provided or present in SKIP_LIST). Only output "SKIP" if every command
	# would be skipped.
	if echo "$@" | tr ' ' '\n' | awk -v run_list="${RUN_LIST:-}" -v skip_list="${SKIP_LIST:-}" '
		function is_skipped(cmd, i, n, parts) {
			if (run_list != "") {
				n = split(run_list, parts, /,/)
				for (i = 1; i <= n; i++) if (parts[i] == cmd) { break }
				if (i > n) return 1
			}
			n = split(skip_list, parts, /,/)
			for (i = 1; i <= n; i++) if (parts[i] == cmd) return 1
			return 0
		}
		BEGIN { all_skip = 1 }
		{ if (!is_skipped($0)) { all_skip = 0 } }
		END { exit (NR == 0 || !all_skip) }
	'; then
		echo "SKIP"
		exit 1
	fi
	return
}
