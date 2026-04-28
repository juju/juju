# Parallel test execution framework.
#
# Provides a queue-based API that mirrors the sequential `run` function from
# run.sh. When parallel mode is active, `run_parallel` queues test names to
# a file (surviving subshell boundaries). `wait_parallel` then executes all
# queued tests concurrently with isolated JUJU_DATA directories so that
# "current model" state does not conflict. All tests share the same
# bootstrapped controller.
#
# Concurrency is limited to TEST_PARALLEL_JOBS (default: 5) to prevent
# resource starvation on the host.
#
# Usage (in a suite's task.sh):
#   parallel_init
#   run() { run_parallel "$@"; }
#   test_group_one   # calls run "func" which now queues
#   test_group_two
#   unset -f run     # restore original
#   wait_parallel    # execute everything
#
# Prerequisites:
#   - A controller must already be bootstrapped
#   - JUJU_DATA (or ~/.local/share/juju) must contain valid client state
#
# Enable via: ./main.sh -P <suite> or TEST_PARALLEL=true ./main.sh <suite>
# Control concurrency: TEST_PARALLEL_JOBS=8 ./main.sh -P <suite>

PARALLEL_QUEUE_FILE=""

# parallel_init prepares the file-based queue. Must be called before
# run_parallel.
parallel_init() {
	PARALLEL_QUEUE_FILE="${TEST_DIR}/parallel-queue-$$"
	rm -f "${PARALLEL_QUEUE_FILE}"
	touch "${PARALLEL_QUEUE_FILE}"
}

# run_parallel queues a test function for concurrent execution by
# wait_parallel. Designed as a drop-in replacement for `run` — same
# single-argument interface. Respects RUN_SUBTEST filtering.
# Uses a file-based queue so it works across subshell boundaries.
run_parallel() {
	local cmd="${1}"

	if [[ -n ${RUN_SUBTEST:-} ]]; then
		# shellcheck disable=SC2143
		if [[ ! "$(echo "${RUN_SUBTEST}" | grep -E "^${cmd}$")" ]]; then
			echo "SKIPPING: ${cmd}"
			return 0
		fi
	fi

	echo "${cmd}" >>"${PARALLEL_QUEUE_FILE}"
}

# wait_parallel executes all queued tests concurrently (up to
# TEST_PARALLEL_JOBS at a time). Each test runs in a subshell with its own
# snapshot of JUJU_DATA, preventing "current model" conflicts. Tests are
# launched in batches; each batch must fully complete before the next starts.
# Returns non-zero if any test failed.
wait_parallel() {
	if [[ ! -s "${PARALLEL_QUEUE_FILE}" ]]; then
		rm -f "${PARALLEL_QUEUE_FILE}"
		return 0
	fi

	local max_jobs="${TEST_PARALLEL_JOBS:-5}"
	local base_juju_data="${JUJU_DATA:-${HOME}/.local/share/juju}"
	local parallel_dir="${TEST_DIR}/parallel-$$"
	mkdir -p "${parallel_dir}"

	# Read queue from file into array.
	local -a queue=()
	while IFS= read -r line; do
		[[ -n "${line}" ]] && queue+=("${line}")
	done <"${PARALLEL_QUEUE_FILE}"
	rm -f "${PARALLEL_QUEUE_FILE}"

	local total=${#queue[@]}
	if [[ ${total} -eq 0 ]]; then
		return 0
	fi

	echo "==> Running ${total} tests in parallel (max ${max_jobs} concurrent)"

	local -a all_cmds=()
	local -a all_iso_dirs=()
	local -a all_statuses=()
	local idx=0

	# Launch tests in batches of max_jobs, waiting for each batch to
	# complete before starting the next.
	while [[ ${idx} -lt ${total} ]]; do
		local remaining=$((total - idx))
		local batch_size=${max_jobs}
		if [[ ${remaining} -lt ${batch_size} ]]; then
			batch_size=${remaining}
		fi

		local -a batch_pids=()

		for ((j = 0; j < batch_size; j++)); do
			local cmd="${queue[$((idx + j))]}"
			local iso_dir="${parallel_dir}/${cmd}"
			mkdir -p "${iso_dir}/juju-data"

			# Snapshot the current juju client state for this test slot.
			cp -a "${base_juju_data}/." "${iso_dir}/juju-data/"

			local desc
			desc=$(echo "${cmd}" | sed -E "s/^run_//g" | sed -E "s/_/ /g")
			echo "===> [   ] Starting: ${desc}"

			# Launch the test in a subshell with isolated JUJU_DATA.
			# If the test fails (via set -e), the subshell exits
			# immediately and duration is not written.
			(
				export JUJU_DATA="${iso_dir}/juju-data"
				set_verbosity

				START_TIME=$(date +%s)
				"${cmd}"
				END_TIME=$(date +%s)

				echo "$((END_TIME - START_TIME))" >"${iso_dir}/duration"
			) >"${iso_dir}/output.log" 2>&1 &

			batch_pids+=($!)
			all_cmds+=("${cmd}")
			all_iso_dirs+=("${iso_dir}")
		done

		# Wait for every job in this batch and record exit status.
		for pid in "${batch_pids[@]}"; do
			if wait "${pid}" 2>/dev/null; then
				all_statuses+=("0")
			else
				all_statuses+=("1")
			fi
		done

		idx=$((idx + batch_size))
	done

	# Report results for all tests.
	local failed=0
	for i in "${!all_cmds[@]}"; do
		local cmd="${all_cmds[$i]}"
		local iso_dir="${all_iso_dirs[$i]}"
		local status="${all_statuses[$i]}"
		local desc
		desc=$(echo "${cmd}" | sed -E "s/^run_//g" | sed -E "s/_/ /g")

		local duration="?"
		if [[ -f "${iso_dir}/duration" ]]; then
			duration=$(cat "${iso_dir}/duration")
		fi

		if [[ "${status}" == "0" ]]; then
			echo -e "===> [ $(green "✔") ] Success: ${desc} (${duration}s)"
		else
			echo -e "===> [ $(red "x") ] Fail: ${desc} (${duration}s)"
			echo "     --- output (last 50 lines) ---"
			tail -50 "${iso_dir}/output.log" | sed 's/^/     | /g'
			echo "     --- end output ---"
			failed=1
		fi
	done

	if [[ ${failed} -ne 0 ]]; then
		return 1
	fi
}
