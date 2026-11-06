# Phase timing for the integration test harness.
#
# Instrumented helpers (bootstrap, pack_charm, wait_for, destroy_model,
# destroy_controller, run, ...) record how long each phase took in
# ${TEST_DIR}/timings.tsv. The file is part of the artifact tarball (-a), and
# a per-phase summary is printed by timing_finish when the run ends.
#
# Columns (tab separated, with a header line):
#
#   suite   test suite, e.g. model
#   test    run_* subtest, or the test_* function outside of a subtest
#   phase   bootstrap, add_model, pack_charm, wait_for, destroy_model,
#           introspect, destroy_controller or subtest
#   detail  what the phase acted on (charm, model, controller, cloud...)
#   seconds wall-clock duration, millisecond resolution
#   result  ok, error (a teardown problem that was tolerated), fail, or
#           stopped (a background teardown no longer waited for)
#   start   start time, seconds since the epoch
#
# A test that fails exits from inside its phases (errexit, wait_timed_out),
# often in a subshell, so nothing gets to record their end. To survive that,
# timing_start leaves a marker file in ${TEST_DIR}/.timings-open and
# timing_end removes it. timing_flush_open, called from the cleanup trap,
# records any markers still present as "fail", timed up to the moment of
# the flush.
#
# These helpers must never write to stdout (pack_charm returns its result on
# stdout) and must never fail, as the test suite runs with errexit.
#
# ```
# local timing_id
# timing_start timing_id <phase> [<detail>]
# ...
# timing_end "${timing_id}" [ok|error|fail]
# ```

TIMING_HEADER=$'suite\ttest\tphase\tdetail\tseconds\tresult\tstart'

# timing_init creates the timings file. It is called by main.sh once
# TEST_DIR exists; until then every other helper here is a no-op.
timing_init() {
	[[ -n ${TEST_DIR:-} ]] || return 0
	mkdir -p "${TEST_DIR}/.timings-open" || true
	echo "${TIMING_HEADER}" >"${TEST_DIR}/timings.tsv" || true
}

# timing_start <var> <phase> [<detail>] starts timing a phase and stores its
# id in the variable named <var>, to be passed to timing_end.
timing_start() {
	# The locals are prefixed so they cannot shadow the caller's variable,
	# which is assigned through printf -v.
	local __ts_var=${1} __ts_phase=${2} __ts_detail=${3:-}
	local __ts_now __ts_id __ts_suite __ts_test

	printf -v "${__ts_var}" '%s' ""
	_timing_enabled || return 0

	_timing_now_us __ts_now
	__ts_id="${__ts_now}-${BASHPID}-${RANDOM}"
	__ts_suite=${TEST_CURRENT_NAME:-}
	__ts_test=${TIMING_TEST:-${TEST_CURRENT:-}}
	__ts_detail=${__ts_detail//[$'\t\n']/ }

	# One field per line: read with a tab IFS would merge empty fields.
	printf '%s\n' "${__ts_suite}" "${__ts_test}" "${__ts_phase}" \
		"${__ts_detail}" "${__ts_now}" \
		>"${TEST_DIR}/.timings-open/${__ts_id}" || return 0
	printf -v "${__ts_var}" '%s' "${__ts_id}"
}

# timing_end <id> [<result>] records the phase started by timing_start.
# The result defaults to "ok".
timing_end() {
	local id=${1:-} result=${2:-ok} file now
	local -a fields

	[[ -n ${id} ]] || return 0
	_timing_enabled || return 0
	file="${TEST_DIR}/.timings-open/${id}"
	[[ -f ${file} ]] || return 0

	_timing_now_us now
	mapfile -t fields <"${file}" || true
	rm -f "${file}" || true
	[[ ${#fields[@]} -eq 5 && -n ${fields[4]} ]] || return 0
	_timing_append "${fields[0]}" "${fields[1]}" "${fields[2]}" \
		"${fields[3]}" "${fields[4]}" "${now}" "${result}"
}

# timing_flush_open records every phase that was started but never ended,
# that is, a phase the test exited from, with the result "fail". Phases of
# processes still running (such as a background model destroy) are left for
# those processes to end.
timing_flush_open() {
	local file id owner

	_timing_enabled || return 0
	for file in "${TEST_DIR}"/.timings-open/*; do
		[[ -f ${file} ]] || continue
		id=$(basename "${file}")
		# The id is <start>-<owner pid>-<random>.
		owner=${id#*-}
		owner=${owner%%-*}
		if [[ ${owner} != "${BASHPID}" ]] && kill -0 "${owner}" 2>/dev/null; then
			continue
		fi
		timing_end "${id}" fail
	done
}

# timing_finish stops timing and prints the total, count and maximum time
# spent in each phase, slowest first.
timing_finish() {
	local file="${TEST_DIR:-}/timings.tsv"

	[[ -n ${TEST_DIR:-} ]] || return 0
	rmdir "${TEST_DIR}/.timings-open" 2>/dev/null || true
	[[ -f ${file} ]] || return 0
	[[ $(wc -l <"${file}") -gt 1 ]] || return 0

	echo "==> Phase timings (seconds, from $(basename "${file}"))"
	{
		printf '%s\t%s\t%s\t%s\t%s\n' phase count total max failed
		awk -F'\t' 'NR > 1 {
			n[$3]++; t[$3] += $5
			if ($5 > m[$3]) m[$3] = $5
			if ($6 == "fail") f[$3]++
		}
		END {
			for (p in n) printf "%s\t%d\t%.1f\t%.1f\t%d\n", p, n[p], t[p], m[p], f[p]
		}' "${file}" | sort -t$'\t' -k3 -gr
	} | column -t -s$'\t' | sed 's/^/    /' || true
}

# _timing_append <suite> <test> <phase> <detail> <start_us> <end_us> <result>
_timing_append() {
	local elapsed
	elapsed=$((${6} - ${5}))
	printf '%s\t%s\t%s\t%s\t%d.%03d\t%s\t%d\n' \
		"${1}" "${2}" "${3}" "${4}" \
		$((elapsed / 1000000)) $(((elapsed % 1000000) / 1000)) \
		"${7}" $((${5} / 1000000)) \
		>>"${TEST_DIR}/timings.tsv" || true
}

# _timing_now_us stores the current time in microseconds since the epoch in
# the named variable, without forking where bash allows it.
_timing_now_us() {
	if [[ -n ${EPOCHREALTIME:-} ]]; then
		# Strip the decimal separator, which depends on the locale.
		printf -v "${1}" '%s' "${EPOCHREALTIME//[^0-9]/}"
	else
		printf -v "${1}" '%s' "$(date +%s%6N)"
	fi
}

_timing_enabled() {
	[[ -n ${TEST_DIR:-} && -d "${TEST_DIR}/.timings-open" ]]
}
