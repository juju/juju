# Scenario harness for tests/includes/gce.sh (GCE region probing and
# region fallback).
#
# Sourced by main.sh as part of the static_analysis suite, which calls
# test_gce_fallback. Each scenario then runs in a separate bash process
# (this file re-executed as a script), so that the `exit 1` paths of
# gce_run_with_region_fallback are observable and one scenario cannot
# poison another's environment. A fixture gcloud serves local files only;
# it never touches the network or credentials.

# Absolute path to this file, resolved at source time: main.sh sources it
# with CWD=tests/, so the relative BASH_SOURCE path resolves here. The
# harness re-executes this file as a script (one process per scenario) and
# cannot resolve it at run time, because test subshells may have cd'd
# elsewhere by then.
GCE_FALLBACK_SELF="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"

test_gce_fallback() {
	if [ "$(skip 'test_gce_fallback')" ]; then
		echo "==> TEST SKIPPED: gce fallback harness"
		return
	fi

	(
		set_verbosity

		# Check the GCE region fallback helpers behave as declared.
		# Unlike most linters this harness is self-contained (it only needs
		# GCE_FALLBACK_SELF and mktemp dirs), so it does not cd to the
		# repository root first.
		run_linter "run_gce_fallback_harness"
	)
}

# run_gce_fallback_harness runs every scenario in a separate bash process
# and reports PASS/FAIL per scenario. Evidence is kept under a temporary
# directory, which is preserved and printed on failure.
run_gce_fallback_harness() {
	local self tmp name d
	local pass=0 failn=0
	local failed=""

	self="${GCE_FALLBACK_SELF}"
	tmp=$(mktemp -d)

	# The fake gcloud serves local fixtures from $GCE_FAKE_DIR. It never
	# performs network access, never reads credentials and never mutates
	# anything outside $GCE_FAKE_DIR.
	mkdir -p "${tmp}/bin"
	cat >"${tmp}/bin/gcloud" <<'FAKE_GCLOUD'
#!/usr/bin/env bash
set -u
die() { echo "fake-gcloud: unhandled: $*" >&2; exit 2; }
[[ ${GCE_FAKE_DIR:-} ]] || die "GCE_FAKE_DIR not set"
[[ $# -ge 3 ]] || die "too few args: $*"

if [[ $1 == compute && $2 == regions && $3 == describe ]]; then
	region=$4
	shift 4
	fmt=""
	for a in "$@"; do
		[[ $a == --format=* ]] && fmt=${a#--format=}
	done
	[[ $fmt == json ]] || die "unhandled --format=$fmt"
	if [[ -f $GCE_FAKE_DIR/regions-describe-fail/$region ]]; then
		echo "ERROR: (gcloud.compute.regions.describe) transient API failure (fixture)" >&2
		exit 1
	fi
	status=$(cat "$GCE_FAKE_DIR/status/$region" 2>/dev/null || echo "")
	printf '{"status":"%s","quotas":[' "${status}"
	sep=""
	if [[ -f $GCE_FAKE_DIR/quotas/$region ]]; then
		while read -r metric limit usage; do
			[[ -n ${metric:-} ]] || continue
			printf '%s{"metric":"%s","limit":%s,"usage":%s}' "${sep}" "${metric}" "${limit}" "${usage}"
			sep=","
		done <"$GCE_FAKE_DIR/quotas/$region"
	fi
	printf ']}\n'
	exit 0
fi

if [[ $1 == compute && $2 == machine-types && $3 == list ]]; then
	if [[ -f $GCE_FAKE_DIR/machine-types-fail ]]; then
		echo "ERROR: (gcloud.compute.machine-types.list) transient API failure (fixture)" >&2
		exit 1
	fi
	mt=""
	for a in "$@"; do
		[[ $a == --filter=name=* ]] && mt=${a#--filter=name=}
	done
	[[ -n $mt ]] || die "no --filter=name= arg in: $*"
	cat "$GCE_FAKE_DIR/machine-types/$mt" 2>/dev/null || true
	exit 0
fi

die "unhandled subcommand: $*"
FAKE_GCLOUD
	chmod +x "${tmp}/bin/gcloud"
	export GCE_HARNESS_BIN="${tmp}/bin"

	# Keep this list ordered and in sync with the gcefb_scenario_*
	# functions defined for the scenario runner below.
	local scenarios="
region_down
machine_type_missing
quota_insufficient
quota_metric_missing
quota_boundary_raw_fraction
malformed_requirement_rejected
api_error_describe
api_error_machine_types
unset_requirements_notice
preset_exclusion
no_usable_region
bootstrap_region_exported
classifier_signatures
infra_failure_retry
bootstrap_infra_failure
genuine_failure_no_retry
infra_then_genuine_no_retry
arch_signature_not_retried
subtest_log_signature
max_attempts
"

	# Word splitting of the scenario list is intended.
	# shellcheck disable=SC2086
	echo "==> Running gce.sh scenario harness ($(echo ${scenarios} | wc -w) scenarios)"
	# shellcheck disable=SC2086
	for name in ${scenarios}; do
		d="${tmp}/${name}"
		mkdir -p "${d}"
		if bash "${self}" --run-scenario "${name}" "${d}" >"${d}/stdout.txt" 2>"${d}/stderr.txt"; then
			printf '    | %-34s PASS\n' "${name}"
			pass=$((pass + 1))
		else
			printf '    | %-34s FAIL\n' "${name}"
			failn=$((failn + 1))
			failed="${failed} ${name}"
		fi
	done
	echo "==> gce.sh scenarios: PASS=${pass} FAIL=${failn}"
	if [[ ${failn} -gt 0 ]]; then
		# shellcheck disable=SC2086
		for name in ${failed}; do
			echo "    --- FAIL ${name} ---"
			if [[ -f ${tmp}/${name}/scenario.out ]]; then
				sed 's/^/    | /g' "${tmp}/${name}/scenario.out"
			fi
			sed 's/^/    | /g' "${tmp}/${name}/stdout.txt"
			sed 's/^/    | /g' "${tmp}/${name}/stderr.txt"
		done
		echo "==> scenario evidence kept at ${tmp}"
		exit 1
	fi
	rm -rf "${tmp}"
}

# __gce_harness_run_scenario sets up an isolated scenario environment
# (fixture gcloud on PATH, stubbed framework integration points, fixture
# and assertion helpers, scenarios) and runs the named scenario. Only used
# when this file is executed as a script; everything is defined inside
# this function so that sourcing the file stays side-effect free.
__gce_harness_run_scenario() {
	local scenario=${1} dir=${2} tests_dir
	tests_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)

	export GCE_FAKE_DIR="${dir}/fake"
	export GCE_SCENARIO_DIR="${dir}"
	export VERBOSE=1
	export TERM=dumb # deterministic, colour-free output
	export TEST_DIR="${dir}/testdir"
	export TEST_CURRENT="cloud_gce"
	mkdir -p "${TEST_DIR}" "${GCE_FAKE_DIR}/status" \
		"${GCE_FAKE_DIR}/quotas" "${GCE_FAKE_DIR}/machine-types" \
		"${GCE_FAKE_DIR}/regions-describe-fail"
	export PATH="${GCE_HARNESS_BIN:?GCE_HARNESS_BIN must point at the fixture gcloud}:${PATH}"

	# Code under test and its real colour/verbosity helpers.
	# shellcheck disable=SC1090
	. "${tests_dir}/includes/colors.sh"
	# shellcheck disable=SC1090
	. "${tests_dir}/includes/verbose.sh"
	# shellcheck disable=SC1090
	. "${tests_dir}/includes/gce.sh"

	# ---------- call records ----------

	BOOTSTRAP_CALLS="${dir}/bootstrap-calls.txt"
	DESTROY_CALLS="${dir}/destroy-calls.txt"
	BODY_CALLS="${dir}/body-calls.txt"
	export BOOTSTRAP_CALLS DESTROY_CALLS BODY_CALLS
	: >"${BOOTSTRAP_CALLS}"
	: >"${DESTROY_CALLS}"
	: >"${BODY_CALLS}"

	# ---------- fixture helpers (drive the fake gcloud) ----------

	fixture_region_status() { # <region> <UP|DOWN>
		printf '%s\n' "$2" >"${GCE_FAKE_DIR}/status/$1"
	}

	fixture_region_quotas() { # <region> "<metric> <limit> <usage>"...
		local region=$1 line
		shift
		: >"${GCE_FAKE_DIR}/quotas/${region}"
		for line in "$@"; do
			printf '%s\n' "${line}" >>"${GCE_FAKE_DIR}/quotas/${region}"
		done
	}

	fixture_machine_type() { # <machine type> <zone>... (appends zones)
		local mt=$1 z
		shift
		touch "${GCE_FAKE_DIR}/machine-types/${mt}"
		for z in "$@"; do
			printf 'https://www.googleapis.com/compute/v1/projects/fake-project/zones/%s\n' "${z}" >>"${GCE_FAKE_DIR}/machine-types/${mt}"
		done
	}

	# A region that satisfies all default requirements.
	fixture_good_region() { # <region> <zone>
		fixture_region_status "$1" UP
		fixture_machine_type g2-standard-4 "$2"
		fixture_region_quotas "$1" "CPUS 24 10" "NVIDIA_L4_GPUS 2 1" "IN_USE_ADDRESSES 8 2"
	}

	set_default_requirements() {
		GCE_REGION_CANDIDATES="${GCE_REGION_CANDIDATES:-us-east1 us-east4}"
		GCE_REQUIRED_MACHINE_TYPES="g2-standard-4"
		GCE_REQUIRED_QUOTAS="CPUS>=6 NVIDIA_L4_GPUS>=1 IN_USE_ADDRESSES>=2"
		export GCE_REGION_CANDIDATES GCE_REQUIRED_MACHINE_TYPES GCE_REQUIRED_QUOTAS
	}

	# ---------- stubs for framework integration points ----------

	bootstrap() {
		local model=$1 logfile=$2 r
		echo "bootstrap model=${model} region=${BOOTSTRAP_REGION:-} reuse=${BOOTSTRAP_REUSE:-UNSET} excluded=[${GCE_EXCLUDED_REGIONS:-}]" >>"${BOOTSTRAP_CALLS}"
		# Word splitting of the failing region list is intended.
		# shellcheck disable=SC2086
		for r in ${GCE_BOOTSTRAP_FAIL_REGIONS:-}; do
			if [[ $r == "${BOOTSTRAP_REGION}" ]]; then
				printf '%s\n' "${GCE_BOOTSTRAP_SIGNATURE:?}" >>"${logfile}"
				echo "bootstrap failing in region ${BOOTSTRAP_REGION}" >>"${BOOTSTRAP_CALLS}"
				return 1
			fi
		done
		echo "bootstrap ok region=${BOOTSTRAP_REGION}" >>"${logfile}"
		return 0
	}

	destroy_controller() {
		echo "destroy_controller region=${BOOTSTRAP_REGION:-}" >>"${DESTROY_CALLS}"
		return 0
	}

	fake_body() { # the "suite command" passed to gce_run_with_region_fallback
		local r target
		echo "body called region=${BOOTSTRAP_REGION:-} excluded=[${GCE_EXCLUDED_REGIONS:-}]" >>"${BODY_CALLS}"
		echo "body output region=${BOOTSTRAP_REGION:-}" >>"${TEST_DIR}/${TEST_CURRENT}.log"
		# Word splitting of the failing region list is intended.
		# shellcheck disable=SC2086
		for r in ${GCE_BODY_FAIL_REGIONS:-}; do
			if [[ $r == "${BOOTSTRAP_REGION}" ]]; then
				target="${GCE_BODY_SIGNATURE_LOG:-${TEST_DIR}/${TEST_CURRENT}.log}"
				printf '%s\n' "${GCE_BODY_SIGNATURE:?}" >>"${target}"
				echo "body failing in region ${BOOTSTRAP_REGION}" >>"${BODY_CALLS}"
				return 1
			fi
		done
		return 0
	}

	# ---------- assertions ----------

	ok() { echo "  ok: $*"; }
	fail() {
		echo "  SCENARIO FAIL: $*" >&2
		exit 1
	}

	assert_eq() { # <got> <want> <what>
		if [[ $1 != "$2" ]]; then
			fail "$3: got [$1] want [$2]"
		fi
		ok "$3 == [$1]"
	}

	assert_contains() { # <got> <want> <what>
		if [[ $1 != *"$2"* ]]; then
			fail "$3: [$1] does not contain [$2]"
		fi
		ok "$3 contains [$2]"
	}

	assert_file_contains() { # <file> <literal> <what>
		if ! grep -qF -- "$2" "$1" 2>/dev/null; then
			fail "$3: file $1 missing literal [$2]"
		fi
		ok "$3: $(basename "$1") contains [$2]"
	}

	assert_file_not_contains() { # <file> <literal> <what>
		if grep -qF -- "$2" "$1" 2>/dev/null; then
			fail "$3: file $1 unexpectedly contains [$2]"
		fi
		ok "$3: $(basename "$1") does not contain [$2]"
	}

	assert_file_pattern_nlines() { # <file> <pattern> <n> <what>
		local n
		n=$(grep -c "$2" "$1" 2>/dev/null || true)
		if [[ $n != "$3" ]]; then
			fail "$4: $1 has ${n} lines matching [$2], want $3"
		fi
		ok "$4: $(basename "$1") has ${n} matching line(s)"
	}

	assert_file_exists() { # <file> <what>
		if [[ ! -e $1 ]]; then
			fail "$2: missing file $1"
		fi
		ok "$2: $(basename "$1") exists"
	}

	assert_file_missing() { # <file> <what>
		if [[ -e $1 ]]; then
			fail "$2: unexpected file $1"
		fi
		ok "$2: no $(basename "$1")"
	}

	# Run gce_select_region capturing its output for assertions/evidence.
	run_select() {
		gce_select_region >"${GCE_SCENARIO_DIR}/select.out" 2>&1
		SELECT_RC=$?
		cat "${GCE_SCENARIO_DIR}/select.out"
	}

	# Run the fallback loop capturing its output for assertions/evidence.
	run_fallback() {
		gce_run_with_region_fallback "test-cloud-gce" "${TEST_DIR}/test-cloud-gce.log" fake_body >"${GCE_SCENARIO_DIR}/fallback.out" 2>&1
		FALLBACK_RC=$?
		cat "${GCE_SCENARIO_DIR}/fallback.out"
	}

	# For scenarios where gce_run_with_region_fallback must exit the
	# process: set an EXIT trap running the given check function, then call
	# the loop. The check runs even when the loop exits the shell; the
	# sentinel guards against the loop returning instead of exiting.
	run_fallback_expect_exit() { # <check function>
		: >"${GCE_SCENARIO_DIR}/expect-exit"
		# shellcheck disable=SC2064
		trap "$1" EXIT
		gce_run_with_region_fallback "test-cloud-gce" "${TEST_DIR}/test-cloud-gce.log" fake_body >"${GCE_SCENARIO_DIR}/fallback.out" 2>&1
		rm -f "${GCE_SCENARIO_DIR}/expect-exit"
		echo "  SCENARIO FAIL: gce_run_with_region_fallback returned (rc=$?) instead of exiting" >&2
		exit 1
	}

	# Common prologue for EXIT-trap checks: claim our own output file (the
	# trap inherits the redirect of the call it exited from), verify the
	# loop really exited the process and dump the captured output.
	exit_check_prologue() { # <expected rc>
		local rc=$?
		exec >"${GCE_SCENARIO_DIR}/scenario.out" 2>&1
		if [[ ! -f ${GCE_SCENARIO_DIR}/expect-exit ]]; then
			fail "gce_run_with_region_fallback returned instead of exiting"
		fi
		echo "--- fallback.out (evidence):"
		cat "${GCE_SCENARIO_DIR}/fallback.out"
		assert_eq "${rc}" "$1" "process exit code"
	}

	# ---------- probe scenarios (gce_select_region) ----------

	gcefb_scenario_region_down() {
		set_default_requirements
		fixture_region_status us-east1 DOWN
		fixture_good_region us-east4 us-east4-b
		run_select
		assert_eq "${SELECT_RC}" 0 "select rc"
		assert_eq "${BOOTSTRAP_REGION}" us-east4 "selected region"
		assert_file_contains "${GCE_SCENARIO_DIR}/select.out" "region us-east1: region is not UP" "down reason"
		assert_eq "$(bash -c 'printf %s "${BOOTSTRAP_REGION:-unset}"')" us-east4 "BOOTSTRAP_REGION exported to child processes"
	}

	gcefb_scenario_machine_type_missing() {
		set_default_requirements
		fixture_region_status us-east1 UP
		fixture_machine_type g2-standard-4 us-west1-a # not in us-east1
		fixture_region_quotas us-east1 "CPUS 24 10" "NVIDIA_L4_GPUS 2 1" "IN_USE_ADDRESSES 8 2"
		fixture_good_region us-east4 us-east4-b
		run_select
		assert_eq "${SELECT_RC}" 0 "select rc"
		assert_eq "${BOOTSTRAP_REGION}" us-east4 "selected region"
		assert_file_contains "${GCE_SCENARIO_DIR}/select.out" "region us-east1: machine type g2-standard-4 is not offered in any zone" "machine type reason"
	}

	gcefb_scenario_quota_insufficient() {
		set_default_requirements
		fixture_region_status us-east1 UP
		fixture_machine_type g2-standard-4 us-east1-b
		fixture_region_quotas us-east1 "CPUS 8 6" "NVIDIA_L4_GPUS 2 1" "IN_USE_ADDRESSES 8 2"
		fixture_good_region us-east4 us-east4-b
		run_select
		assert_eq "${SELECT_RC}" 0 "select rc"
		assert_eq "${BOOTSTRAP_REGION}" us-east4 "selected region"
		assert_file_contains "${GCE_SCENARIO_DIR}/select.out" "region us-east1: quota CPUS headroom 2 below required 6" "quota reason"
	}

	gcefb_scenario_quota_metric_missing() {
		set_default_requirements
		fixture_region_status us-east1 UP
		fixture_machine_type g2-standard-4 us-east1-b
		# No CPUS line at all: no grant in this region.
		fixture_region_quotas us-east1 "NVIDIA_L4_GPUS 2 1" "IN_USE_ADDRESSES 8 2"
		fixture_good_region us-east4 us-east4-b
		run_select
		assert_eq "${SELECT_RC}" 0 "select rc"
		assert_eq "${BOOTSTRAP_REGION}" us-east4 "selected region"
		assert_file_contains "${GCE_SCENARIO_DIR}/select.out" "region us-east1: quota CPUS headroom 0 below required 6" "missing metric means no grant"
	}

	gcefb_scenario_quota_boundary_raw_fraction() {
		set_default_requirements
		GCE_REGION_CANDIDATES="us-east1"
		GCE_REQUIRED_QUOTAS="CPUS>=6"
		fixture_region_status us-east1 UP
		fixture_machine_type g2-standard-4 us-east1-b
		# headroom == needed passes.
		fixture_region_quotas us-east1 "CPUS 8 2"
		run_select
		assert_eq "${SELECT_RC}" 0 "boundary: headroom == needed passes"
		assert_eq "${BOOTSTRAP_REGION}" us-east1 "selected region"
		# Fractional usage: headroom 5.9 must fail a >=6 requirement (no
		# rounding to an integer before comparing).
		fixture_region_quotas us-east1 "CPUS 8 2.1"
		unset BOOTSTRAP_REGION
		run_select
		assert_eq "${SELECT_RC}" 1 "fractional headroom below needed fails"
		assert_file_contains "${GCE_SCENARIO_DIR}/select.out" "quota CPUS headroom 5.9 below required 6" "raw fraction compared"
	}

	gcefb_scenario_malformed_requirement_rejected() {
		set_default_requirements
		fixture_good_region us-east1 us-east1-b
		GCE_REGION_CANDIDATES="us-east1"
		local bad
		for bad in "CPUS=6" "CPUS>=" "CPUS>=abc" ">=6"; do
			unset BOOTSTRAP_REGION
			GCE_REQUIRED_QUOTAS="${bad}"
			run_select
			assert_eq "${SELECT_RC}" 1 "malformed ${bad} rejected"
			assert_file_contains "${GCE_SCENARIO_DIR}/select.out" "malformed requirement '${bad}'" "loud diagnostic for ${bad}"
			assert_eq "${BOOTSTRAP_REGION:-unset}" "unset" "no region selected on malformed input"
		done
		# The documented format is accepted.
		unset BOOTSTRAP_REGION
		GCE_REQUIRED_QUOTAS="CPUS>=6"
		run_select
		assert_eq "${SELECT_RC}" 0 "well-formed requirement accepted"
		assert_eq "${BOOTSTRAP_REGION}" us-east1 "selected region"
	}

	gcefb_scenario_api_error_describe() {
		set_default_requirements
		fixture_good_region us-east1 us-east1-b
		fixture_good_region us-east4 us-east4-b
		touch "${GCE_FAKE_DIR}/regions-describe-fail/us-east1"
		run_select
		assert_eq "${SELECT_RC}" 0 "select rc"
		assert_eq "${BOOTSTRAP_REGION}" us-east4 "selected region"
		assert_file_contains "${GCE_SCENARIO_DIR}/select.out" "region us-east1: GCE API error describing region" "API error reported as such"
		assert_file_not_contains "${GCE_SCENARIO_DIR}/select.out" "region us-east1: region is not UP" "API error not misreported as region down"
	}

	gcefb_scenario_api_error_machine_types() {
		set_default_requirements
		fixture_good_region us-east1 us-east1-b
		fixture_good_region us-east4 us-east4-b
		touch "${GCE_FAKE_DIR}/machine-types-fail"
		run_select
		assert_eq "${SELECT_RC}" 1 "no region usable while the API errors"
		assert_file_contains "${GCE_SCENARIO_DIR}/select.out" "GCE API error listing machine types" "API error reported as such"
		assert_file_not_contains "${GCE_SCENARIO_DIR}/select.out" "is not offered in any zone" "API error not misreported as missing offer"
		assert_file_contains "${GCE_SCENARIO_DIR}/select.out" "no usable GCE region for this suite" "fail-fast message"
	}

	gcefb_scenario_unset_requirements_notice() {
		set_default_requirements
		unset GCE_REQUIRED_MACHINE_TYPES GCE_REQUIRED_QUOTAS
		fixture_region_status us-east1 UP
		run_select
		assert_eq "${SELECT_RC}" 0 "any UP region accepted with no requirements"
		assert_file_contains "${GCE_SCENARIO_DIR}/select.out" "note: GCE_REQUIRED_MACHINE_TYPES is unset" "machine type notice"
		assert_file_contains "${GCE_SCENARIO_DIR}/select.out" "note: GCE_REQUIRED_QUOTAS is unset" "quota notice"
	}

	gcefb_scenario_preset_exclusion() {
		set_default_requirements
		fixture_good_region us-east1 us-east1-b
		fixture_good_region us-east4 us-east4-b
		export GCE_EXCLUDED_REGIONS=" us-east1"
		run_select
		assert_eq "${SELECT_RC}" 0 "select rc"
		assert_eq "${BOOTSTRAP_REGION}" us-east4 "selected region"
		assert_file_contains "${GCE_SCENARIO_DIR}/select.out" "region us-east1: excluded after a previous infrastructure failure" "exclusion reason"
	}

	gcefb_scenario_no_usable_region() {
		set_default_requirements
		fixture_region_status us-east1 DOWN
		fixture_region_status us-east4 DOWN
		fixture_machine_type g2-standard-4 us-east1-b us-east4-b
		fixture_region_quotas us-east1 "CPUS 24 10" "NVIDIA_L4_GPUS 2 1" "IN_USE_ADDRESSES 8 2"
		fixture_region_quotas us-east4 "CPUS 24 10" "NVIDIA_L4_GPUS 2 1" "IN_USE_ADDRESSES 8 2"
		run_select
		assert_eq "${SELECT_RC}" 1 "select rc"
		assert_file_contains "${GCE_SCENARIO_DIR}/select.out" "no usable GCE region for this suite" "fail-fast message"
		assert_file_contains "${GCE_SCENARIO_DIR}/select.out" "candidates: us-east1 us-east4" "candidates dumped"
		assert_file_contains "${GCE_SCENARIO_DIR}/select.out" "required machine types: g2-standard-4" "machine types dumped"
		assert_file_contains "${GCE_SCENARIO_DIR}/select.out" "required quotas:" "quotas dumped"
	}

	gcefb_scenario_bootstrap_region_exported() {
		set_default_requirements
		fixture_good_region us-east1 us-east1-b
		run_select
		assert_eq "${SELECT_RC}" 0 "select rc"
		assert_eq "${BOOTSTRAP_REGION}" us-east1 "selected region"
		assert_eq "$(bash -c 'printf %s "${BOOTSTRAP_REGION:-unset}"')" us-east1 "BOOTSTRAP_REGION exported to child processes"
	}

	# ---------- classifier scenario (gce_log_is_infra_failure) ----------

	gcefb_scenario_classifier_signatures() {
		local d="${TEST_DIR}"
		printf 'ERROR: QUOTA_EXCEEDED: limit 0\n' >"${d}/quota.log"
		printf 'ZONE_RESOURCE_POOL_EXHAUSTED in zone us-east1-b\n' >"${d}/stockout.log"
		printf "machineTypes/g2-standard-4' was not found\n" >"${d}/notfound.log"
		printf 'not compatible with machine type architecture\n' >"${d}/arch.log"
		printf 'assertion failed: genuine test failure\n' >"${d}/genuine.log"

		gce_log_is_infra_failure "${d}/quota.log" || fail "QUOTA_EXCEEDED must classify as infra"
		ok "QUOTA_EXCEEDED is infra"
		gce_log_is_infra_failure "${d}/stockout.log" || fail "ZONE_RESOURCE_POOL_EXHAUSTED must classify as infra"
		ok "ZONE_RESOURCE_POOL_EXHAUSTED is infra"
		gce_log_is_infra_failure "${d}/notfound.log" || fail "machineTypes not found must classify as infra"
		ok "machineTypes not found is infra"
		if gce_log_is_infra_failure "${d}/arch.log"; then
			fail "arch mismatch is client-side and must NOT classify as infra"
		fi
		ok "arch mismatch is not infra (client-side, fails fast)"
		if gce_log_is_infra_failure "${d}/genuine.log"; then
			fail "genuine failure must NOT classify as infra"
		fi
		ok "genuine failure is not infra"
	}

	# ---------- fallback scenarios (gce_run_with_region_fallback) ----------

	gcefb_scenario_infra_failure_retry() {
		set_default_requirements
		fixture_good_region us-east1 us-east1-b
		fixture_good_region us-east4 us-east4-b

		# Mimic ./main.sh -r: reuse would otherwise be true; the fallback
		# must force a fresh bootstrap for the retry.
		export BOOTSTRAP_REUSE="true"
		export GCE_BODY_FAIL_REGIONS="us-east1"
		export GCE_BODY_SIGNATURE='ERROR: (ZONE_RESOURCE_POOL_EXHAUSTED) machine creation failed in zone us-east1-b'

		run_fallback
		assert_eq "${FALLBACK_RC}" 0 "overall exit code"
		assert_file_pattern_nlines "${BODY_CALLS}" "body called" 2 "suite body ran twice"
		assert_file_pattern_nlines "${BOOTSTRAP_CALLS}" "bootstrap model=" 2 "bootstrap ran twice"
		assert_file_pattern_nlines "${DESTROY_CALLS}" "." 1 "destroy_controller ran once (after success)"
		assert_file_contains "${BOOTSTRAP_CALLS}" "region=us-east1 reuse=true" "first attempt reused"
		assert_file_contains "${BOOTSTRAP_CALLS}" "region=us-east4 reuse=false excluded=[ us-east1]" "retry ran fresh with us-east1 excluded"
		assert_contains "${GCE_EXCLUDED_REGIONS:-}" "us-east1" "GCE_EXCLUDED_REGIONS grew"
		assert_eq "${BOOTSTRAP_REUSE:-}" "false" "BOOTSTRAP_REUSE forced false for the retry"
		assert_file_contains "${GCE_SCENARIO_DIR}/fallback.out" "GCE infrastructure failure in us-east1" "infra failure classified"
		assert_file_contains "${GCE_SCENARIO_DIR}/fallback.out" "Running suite in region us-east4" "second attempt announced"
		# The failed attempt's logs are archived under its own number.
		assert_file_exists "${TEST_DIR}/test-cloud-gce.log.attempt-1" "attempt-1 bootstrap log archived"
		assert_file_exists "${TEST_DIR}/${TEST_CURRENT}.log.attempt-1" "attempt-1 suite log archived"
		assert_file_contains "${TEST_DIR}/${TEST_CURRENT}.log.attempt-1" "body output region=us-east1" "attempt-1 archive holds attempt 1 content"
		assert_file_contains "${TEST_DIR}/${TEST_CURRENT}.log.attempt-1" "ZONE_RESOURCE_POOL_EXHAUSTED" "attempt-1 archive holds the signature"
	}

	gcefb_scenario_bootstrap_infra_failure() {
		set_default_requirements
		fixture_good_region us-east1 us-east1-b
		fixture_good_region us-east4 us-east4-b
		export GCE_BOOTSTRAP_FAIL_REGIONS="us-east1"
		export GCE_BOOTSTRAP_SIGNATURE='ERROR: QUOTA_EXCEEDED: N2D_CPUS, limit 0'

		run_fallback
		assert_eq "${FALLBACK_RC}" 0 "overall exit code"
		assert_file_pattern_nlines "${BOOTSTRAP_CALLS}" "bootstrap model=" 2 "bootstrap ran twice"
		assert_file_contains "${GCE_SCENARIO_DIR}/fallback.out" "GCE infrastructure failure in us-east1" "bootstrap-time signature classified"
		assert_file_contains "${TEST_DIR}/test-cloud-gce.log.attempt-1" "QUOTA_EXCEEDED" "bootstrap log archived with the signature"
	}

	gcefb_scenario_genuine_failure_no_retry() {
		set_default_requirements
		fixture_good_region us-east1 us-east1-b
		fixture_good_region us-east4 us-east4-b
		export GCE_BODY_FAIL_REGIONS="us-east1"
		export GCE_BODY_SIGNATURE='assertion failed: expected status "started" but got "pending" (genuine test failure)'

		run_fallback_expect_exit gcefb_check_genuine_failure
	}

	gcefb_check_genuine_failure() {
		exit_check_prologue 1
		assert_file_pattern_nlines "${BODY_CALLS}" "body called" 1 "body ran exactly once (no region switch)"
		assert_file_pattern_nlines "${BOOTSTRAP_CALLS}" "bootstrap model=" 1 "bootstrap ran exactly once"
		assert_file_contains "${GCE_SCENARIO_DIR}/fallback.out" "Failure is not a GCE infrastructure issue, not trying another region" "genuine failure message"
		assert_eq "${GCE_EXCLUDED_REGIONS:-}" "" "region NOT excluded on genuine failure"
		assert_file_exists "${TEST_DIR}/${TEST_CURRENT}.log" "current logs left in place for the summary"
		assert_file_missing "${TEST_DIR}/${TEST_CURRENT}.log.attempt-1" "no rotation without a retry"
		exit 0
	}

	gcefb_scenario_infra_then_genuine_no_retry() {
		set_default_requirements
		fixture_good_region us-east1 us-east1-b
		fixture_good_region us-east4 us-east4-b
		export GCE_BODY_FAIL_REGIONS="us-east1 us-east4"
		# Per-region signatures: infra in us-east1, genuine in us-east4.
		export GCE_BODY_SIGNATURE='PLACEHOLDER'

		# Drive the two failures through a body of our own: infra signature
		# in us-east1, genuine message in us-east4.
		fake_body() {
			echo "body called region=${BOOTSTRAP_REGION:-}" >>"${BODY_CALLS}"
			echo "body output region=${BOOTSTRAP_REGION:-}" >>"${TEST_DIR}/${TEST_CURRENT}.log"
			case ${BOOTSTRAP_REGION} in
			us-east1)
				echo "QUOTA_EXCEEDED: CPUS limit reached" >>"${TEST_DIR}/${TEST_CURRENT}.log"
				return 1
				;;
			us-east4)
				echo "assertion failed: genuine failure" >>"${TEST_DIR}/${TEST_CURRENT}.log"
				return 1
				;;
			esac
			return 0
		}

		run_fallback_expect_exit gcefb_check_infra_then_genuine
	}

	gcefb_check_infra_then_genuine() {
		exit_check_prologue 1
		assert_file_pattern_nlines "${BOOTSTRAP_CALLS}" "bootstrap model=" 2 "exactly two attempts"
		assert_file_contains "${GCE_SCENARIO_DIR}/fallback.out" "GCE infrastructure failure in us-east1" "attempt 1 classified infra"
		assert_file_contains "${GCE_SCENARIO_DIR}/fallback.out" "Failure is not a GCE infrastructure issue, not trying another region" "attempt 2 classified genuine"
		assert_eq "${GCE_EXCLUDED_REGIONS:- us-east1}" " us-east1" "only us-east1 excluded"
		# Attempt 1's stale signature must not have poisoned attempt 2's
		# classification (rotation happened between the two).
		assert_file_contains "${TEST_DIR}/${TEST_CURRENT}.log.attempt-1" "QUOTA_EXCEEDED" "attempt-1 archive holds its signature"
		assert_file_contains "${TEST_DIR}/${TEST_CURRENT}.log" "genuine failure" "attempt 2 logs left current"
		assert_file_not_contains "${TEST_DIR}/${TEST_CURRENT}.log" "QUOTA_EXCEEDED" "attempt 2 current log carries no stale signature"
		exit 0
	}

	gcefb_scenario_arch_signature_not_retried() {
		set_default_requirements
		fixture_good_region us-east1 us-east1-b
		fixture_good_region us-east4 us-east4-b
		export GCE_BODY_FAIL_REGIONS="us-east1"
		export GCE_BODY_SIGNATURE='machine image is not compatible with machine type architecture'

		run_fallback_expect_exit gcefb_check_arch_signature
	}

	gcefb_check_arch_signature() {
		exit_check_prologue 1
		assert_file_pattern_nlines "${BOOTSTRAP_CALLS}" "bootstrap model=" 1 "no retry: client-side failure fails fast"
		assert_file_contains "${GCE_SCENARIO_DIR}/fallback.out" "Failure is not a GCE infrastructure issue, not trying another region" "arch mismatch treated as genuine"
		exit 0
	}

	gcefb_scenario_subtest_log_signature() {
		set_default_requirements
		fixture_good_region us-east1 us-east1-b
		fixture_good_region us-east4 us-east4-b
		export GCE_BODY_FAIL_REGIONS="us-east1"
		export GCE_BODY_SIGNATURE='ERROR: QUOTA_EXCEEDED: IN_USE_ADDRESSES during add-model'
		# The signature lands in a subtest-local log, not in the bootstrap
		# or suite summary logs.
		export GCE_BODY_SIGNATURE_LOG="${TEST_DIR}/test-deploy-gpu-instance.log"

		run_fallback
		assert_eq "${FALLBACK_RC}" 0 "overall exit code"
		assert_file_pattern_nlines "${BOOTSTRAP_CALLS}" "bootstrap model=" 2 "retried on a subtest-log signature"
		assert_file_contains "${GCE_SCENARIO_DIR}/fallback.out" "GCE infrastructure failure in us-east1" "subtest-log signature classified"
		assert_file_exists "${TEST_DIR}/test-deploy-gpu-instance.log.attempt-1" "subtest log rotated with the attempt"
	}

	gcefb_scenario_max_attempts() {
		set_default_requirements
		fixture_good_region us-east1 us-east1-b
		fixture_good_region us-east4 us-east4-b
		fixture_good_region us-central1 us-central1-a
		GCE_REGION_CANDIDATES="us-east1 us-east4 us-central1"
		GCE_REQUIRED_QUOTAS="CPUS>=6"
		export GCE_REGION_CANDIDATES GCE_REQUIRED_QUOTAS GCE_MAX_ATTEMPTS=2
		export GCE_BODY_FAIL_REGIONS="us-east1 us-east4 us-central1"
		export GCE_BODY_SIGNATURE='ERROR: QUOTA_EXCEEDED (fixture)'

		run_fallback_expect_exit gcefb_check_max_attempts
	}

	gcefb_check_max_attempts() {
		exit_check_prologue 1
		assert_file_pattern_nlines "${BOOTSTRAP_CALLS}" "bootstrap model=" 2 "exactly GCE_MAX_ATTEMPTS attempts"
		assert_file_contains "${GCE_SCENARIO_DIR}/fallback.out" "2 GCE region attempt(s) exhausted" "budget message"
		assert_file_contains "${GCE_SCENARIO_DIR}/fallback.out" "per-attempt logs:" "evidence pointer"
		exit 0
	}

	"gcefb_scenario_${scenario}"
}

# Executed as a script: run one scenario in this process. When sourced by
# main.sh this branch is inert.
if [[ ${BASH_SOURCE[0]} == "${0}" ]]; then
	if [[ ${1:-} != "--run-scenario" ]]; then
		echo "usage: $0 --run-scenario <name> <dir>" >&2
		exit 2
	fi
	__gce_harness_run_scenario "${2:?scenario name required}" "${3:?scenario dir required}"
fi
