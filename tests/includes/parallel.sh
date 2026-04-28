# Parallel test execution framework.
#
# Provides a queue-based API that mirrors the sequential `run` function from
# run.sh. Tests are queued with `run_parallel` and executed concurrently by
# `wait_parallel`, each with an isolated JUJU_DATA directory so that
# "current model" state does not conflict between concurrent tests. All tests
# share the same bootstrapped controller.
#
# Concurrency is limited to TEST_PARALLEL_JOBS (default: 5) to prevent
# resource starvation on the host.
#
# Usage:
#   run_parallel "run_deploy_charm"
#   run_parallel "run_deploy_bundle"
#   wait_parallel
#
# Each queued function must follow the standard pattern:
#   - Call ensure "unique-model-name" "${file}" to create its own model
#   - Do work
#   - Call destroy_model "unique-model-name"
#
# Prerequisites:
#   - A controller must already be bootstrapped
#   - JUJU_DATA (or ~/.local/share/juju) must contain valid client state
#
# Enable via: ./main.sh -P <suite> or TEST_PARALLEL=true ./main.sh <suite>
# Control concurrency: TEST_PARALLEL_JOBS=8 ./main.sh -P <suite>

PARALLEL_QUEUE=()

# run_parallel queues a test function for concurrent execution by
# wait_parallel. Respects RUN_SUBTEST filtering, matching the behaviour
# of `run`.
run_parallel() {
	local cmd="${1}"

	if [[ -n ${RUN_SUBTEST:-} ]]; then
		# shellcheck disable=SC2143
		if [[ ! "$(echo "${RUN_SUBTEST}" | grep -E "^${cmd}$")" ]]; then
			echo "SKIPPING: ${cmd}"
			return 0
		fi
	fi

	PARALLEL_QUEUE+=("${cmd}")
}

# wait_parallel executes all queued tests concurrently (up to
# TEST_PARALLEL_JOBS at a time). Each test runs in a subshell with its own
# snapshot of JUJU_DATA, preventing "current model" conflicts. Results are
# reported as each test completes. Returns non-zero if any test failed.
# The queue is cleared after execution.
wait_parallel() {
	if [[ ${#PARALLEL_QUEUE[@]} -eq 0 ]]; then
		return 0
	fi

	local max_jobs="${TEST_PARALLEL_JOBS:-5}"
	local base_juju_data="${JUJU_DATA:-${HOME}/.local/share/juju}"
	local parallel_dir="${TEST_DIR}/parallel-$$"
	mkdir -p "${parallel_dir}"

	local -a pids=()
	local -a cmds=()
	local -a iso_dirs=()
	local -a active_pids=()

	echo "==> Running ${#PARALLEL_QUEUE[@]} tests in parallel (max ${max_jobs} concurrent)"

	for cmd in "${PARALLEL_QUEUE[@]}"; do
		# If at capacity, wait for at least one job to finish.
		while [[ ${#active_pids[@]} -ge ${max_jobs} ]]; do
			wait -n 2>/dev/null || true
			# Rebuild active list — remove finished PIDs.
			local -a still_active=()
			for pid in "${active_pids[@]}"; do
				if kill -0 "${pid}" 2>/dev/null; then
					still_active+=("${pid}")
				fi
			done
			active_pids=("${still_active[@]}")
		done

		local iso_dir="${parallel_dir}/${cmd}"
		mkdir -p "${iso_dir}/juju-data"

		# Snapshot the current juju client state for this test slot.
		cp -a "${base_juju_data}/." "${iso_dir}/juju-data/"

		local desc
		desc=$(echo "${cmd}" | sed -E "s/^run_//g" | sed -E "s/_/ /g")
		echo "===> [   ] Starting: ${desc}"

		# Launch the test in a subshell with isolated JUJU_DATA.
		(
			export JUJU_DATA="${iso_dir}/juju-data"
			set_verbosity

			START_TIME=$(date +%s)
			"${cmd}"
			END_TIME=$(date +%s)

			echo "$((END_TIME - START_TIME))" >"${iso_dir}/duration"
		) >"${iso_dir}/output.log" 2>&1 &

		local launched_pid=$!
		pids+=("${launched_pid}")
		active_pids+=("${launched_pid}")
		cmds+=("${cmd}")
		iso_dirs+=("${iso_dir}")
	done

	# Clear the queue for reuse.
	PARALLEL_QUEUE=()

	# Wait for all tests to complete and collect results.
	local failed=0
	for i in "${!pids[@]}"; do
		local pid="${pids[$i]}"
		local cmd="${cmds[$i]}"
		local iso_dir="${iso_dirs[$i]}"
		local desc
		desc=$(echo "${cmd}" | sed -E "s/^run_//g" | sed -E "s/_/ /g")

		if wait "${pid}"; then
			local duration="?"
			if [[ -f "${iso_dir}/duration" ]]; then
				duration=$(cat "${iso_dir}/duration")
			fi
			echo -e "===> [ $(green "✔") ] Success: ${desc} (${duration}s)"
		else
			echo -e "===> [ $(red "x") ] Fail: ${desc}"
			echo "     --- output ---"
			sed 's/^/     | /g' "${iso_dir}/output.log"
			echo "     --- end output ---"
			failed=1
		fi
	done

	if [[ ${failed} -ne 0 ]]; then
		return 1
	fi
}
