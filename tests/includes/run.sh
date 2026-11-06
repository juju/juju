# run_concurrently <subtest> starts a subtest like run does, but in the
# background, so subtests that wait on their own machines overlap. Call
# wait_for_subtests before anything that depends on them. At most
# SUBTEST_CONCURRENCY (default 4) run at once; 1 runs them one by one, as
# run does.
#
# Each subtest gets its own copy of the client store (JUJU_DATA), so it has
# its own current model on the shared controller. Only use it for subtests
# that work in their own models and neither bootstrap nor destroy
# controllers: a controller bootstrapped in a copy is unknown to cleanup.
#
# The copy holds credentials, so it lives outside TEST_DIR, which is
# archived as the test artifact. It is removed at the end of the run
# (remove_subtest_scratch_dirs), as the subtest's background model destroys
# still use it after the subtest has finished.
#
# ```
# run_concurrently "run_one"
# run_concurrently "run_two"
# wait_for_subtests
# ```
run_concurrently() {
	local cmd scratch file

	cmd=${1}
	if [[ ${SUBTEST_CONCURRENCY:-4} -le 1 || -n ${RUN_SUBTEST} ]]; then
		run "$@"
		return
	fi

	file="${TEST_DIR}/concurrent-subtests"
	touch "${file}"
	while [[ $(wc -l <"${file}") -ge ${SUBTEST_CONCURRENCY:-4} ]]; do
		_wait_for_any_subtest
	done

	scratch=$(mktemp -d "${TMPDIR:-/tmp}/juju-subtest-${cmd}.XXXXXX")
	cp -a "${JUJU_DATA:-${HOME}/.local/share/juju}" "${scratch}/juju-data"
	(
		export JUJU_DATA="${scratch}/juju-data"
		# pack_charm copies charms here.
		export SUBTEST_SCRATCH_DIR="${scratch}"
		# Its own log file, as TEST_CURRENT names the run output.
		TEST_CURRENT="${TEST_CURRENT}-${cmd}"
		# Its own daemon scope, so that it only stops its own daemons.
		# shellcheck disable=SC2034
		DAEMON_SCOPE_ROOT="scope_index_${cmd}"
		run "$@"
	) </dev/null >"${TEST_DIR}/${cmd}.concurrent.log" 2>&1 &
	echo "$! ${cmd} ${scratch}" >>"${file}"
	echo "${scratch}" >>"${TEST_DIR}/subtest-scratch-dirs"
	echo "===> [ & ] Started: $(echo "${cmd}" | sed -E "s/^run_//g; s/_/ /g")"
}

# wait_for_subtests waits for every subtest run_concurrently started, and
# fails the test if any of them failed.
wait_for_subtests() {
	local file

	file="${TEST_DIR}/concurrent-subtests"
	while [[ -s ${file} ]]; do
		_wait_for_any_subtest
	done
}

# stop_concurrent_subtests stops every subtest run_concurrently started.
stop_concurrent_subtests() {
	local file pid cmd scratch

	file="${TEST_DIR}/concurrent-subtests"
	[[ -s ${file} ]] || return 0
	while read -r pid cmd scratch; do
		pkill -P "${pid}" 2>/dev/null || true
		kill "${pid}" 2>/dev/null || true
	done <"${file}"
	: >"${file}"
}

# remove_subtest_scratch_dirs removes the scratch directories (copies of the
# client store, charms) of every subtest run_concurrently started. Call it
# once nothing uses them any more: after cleanup_jujus has waited for the
# background model destroys.
remove_subtest_scratch_dirs() {
	local file dir

	file="${TEST_DIR}/subtest-scratch-dirs"
	[[ -s ${file} ]] || return 0
	while read -r dir; do
		rm -rf "${dir}"
	done <"${file}"
	: >"${file}"
}

# _wait_for_any_subtest waits for the next subtest to finish, prints its
# result and exits if it failed.
_wait_for_any_subtest() {
	local file pid cmd result done_pid
	local -a pids

	file="${TEST_DIR}/concurrent-subtests"
	mapfile -t pids < <(awk '{ print $1 }' "${file}")
	result=0
	wait -n -p done_pid "${pids[@]}" || result=$?

	read -r pid cmd _ < <(awk -v pid="${done_pid}" '$1 == pid' "${file}")
	sed -i "/^${done_pid} /d" "${file}"

	grep -E 'Success:|Fail:' "${TEST_DIR}/${cmd}.concurrent.log" |
		sed 's/^.*===>/===>/' || true
	if [[ ${result} -ne 0 ]]; then
		echo "===> [ $(red "x") ] Fail: ${cmd} (exit ${result})"
		# cleanup prints the test's log when it fails.
		{
			echo "==> ${cmd}"
			cat "${TEST_DIR}/${cmd}.concurrent.log"
			cat "${TEST_DIR}/${TEST_CURRENT}-${cmd}.log" 2>/dev/null || true
		} >>"${TEST_DIR}/${TEST_CURRENT}.log"
		# The test has failed: stop the others rather than leave them
		# running while the controller is torn down.
		stop_concurrent_subtests
		exit "${result}"
	fi
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

	# Dynamically scoped, so phases timed inside this subtest are attributed
	# to it (see timing.sh).
	# shellcheck disable=SC2034
	local TIMING_TEST="${CMD}" timing_id
	timing_start timing_id subtest "${CMD}"
	# Dynamically scoped: destroy_controller still gathers introspection for
	# a teardown called from a subtest.
	# shellcheck disable=SC2034
	local IN_SUBTEST=true

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
	timing_end "${timing_id}"

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

	# shellcheck disable=SC2034
	local TIMING_TEST="${CMD}" timing_id
	timing_start timing_id subtest "${CMD}"

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
		timing_end "${timing_id}"
		echo -e "\r\033[1A\033[0K===> [ $(green "✔") ] Success: ${DESC} ($((END_TIME - START_TIME))s)"
	else
		timing_end "${timing_id}" fail
		echo -e "\r\033[1A\033[0K===> [ $(red "x") ] Fail: ${DESC} ($((END_TIME - START_TIME))s)"
		exit 1
	fi
}

# add a test to the skip list
add_skipped() {
  SKIP_LIST="${SKIP_LIST:+$SKIP_LIST,}${1#,}";
}

skip() {
	# For each command, check if it would be skipped (absent from RUN_LIST when
	# provided or present in SKIP_LIST). Only output "SKIP" if every command
	# would be skipped.
	if echo "$@" | tr ' ' '\n' | awk -v run_list="${RUN_LIST:-}" -v skip_list="${SKIP_LIST:-}" '
		function strip_quotes(s) {
			gsub(/^"+|"+$/, "", s)
			return s
		}
		function is_skipped(cmd, i, n, parts) {
			if (run_list != "") {
				n = split(run_list, parts, /,/)
				for (i = 1; i <= n; i++) if (strip_quotes(parts[i]) == cmd) { break }
				if (i > n) return 1
			}
			n = split(skip_list, parts, /,/)
			for (i = 1; i <= n; i++) if (strip_quotes(parts[i]) == cmd) return 1
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
