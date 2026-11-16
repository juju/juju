# GCE region probing and region fallback helpers.
#
# These helpers let a suite pick a usable GCE region at runtime instead of
# pinning one, so that Google infrastructure issues (out of stock machine
# types, missing quota grants) do not fail CI runs:
#
# - gce_select_region probes the candidate regions in preference order and
#   exports BOOTSTRAP_REGION with the first region that satisfies the
#   suite's declared requirements, failing fast when none does.
# - gce_run_with_region_fallback wraps a suite body: when the suite fails
#   for an unambiguous GCE infrastructure reason, the region is excluded
#   and the suite is retried in the next usable region.
#
# Requirements are declared by the suite through these variables:
#   GCE_REGION_CANDIDATES      ordered, space separated region names
#   GCE_REQUIRED_MACHINE_TYPES space separated machine types that must be
#                              offered in at least one zone of the region
#   GCE_REQUIRED_QUOTAS        space separated "<METRIC>>=<needed>" pairs,
#                              checked against the region's live limit/usage
#   GCE_EXCLUDED_REGIONS       space separated regions to skip (set by the
#                              fallback loop, may also be preset)
#
# An empty/unset GCE_REQUIRED_* variable disables that check entirely;
# suites should set them explicitly.
#
# GCE_MAX_ATTEMPTS caps how many regions the fallback loop tries before
# failing fast (default 3).

# gce_machine_type_available reports whether a machine type (e.g.
# g2-standard-4) is offered in at least one zone of the given region.
#
# ```
# gce_machine_type_available <region> <machine type>
# ```
gce_machine_type_available() {
	local region machine_type zones
	region=${1}
	machine_type=${2}

	# machine-types list is an aggregated API: entries carry their zone as
	# a full URL, so match on "zones/<region>-".
	if ! zones=$(gcloud compute machine-types list \
		--filter="name=${machine_type}" \
		--format="value(zone)" 2>&1); then
		echo "    | region ${region}: GCE API error listing machine types: ${zones}"
		return 1
	fi
	if ! grep -q "zones/${region}-" <<<"${zones}"; then
		echo "    | region ${region}: machine type ${machine_type} is not offered in any zone"
		return 1
	fi
	return 0
}

# gce_region_quota_headroom checks that a region has enough spare quota for
# every "<METRIC>>=<needed>" requirement passed, against the region's quota
# report given as "<metric> <limit> <usage>" lines (from a
# `gcloud compute regions describe` of the region). A metric missing from
# the report means the project has no grant for it in that region at all.
#
# A malformed requirement (anything not matching "<METRIC>>=<needed>") is
# rejected loudly instead of silently passing for every region.
#
# ```
# gce_region_quota_headroom <region> <quota report> <METRIC>>=<needed>...
# ```
gce_region_quota_headroom() {
	local region quotas req metric needed headroom req_re
	region=${1}
	quotas=${2}
	shift 2

	req_re='^[A-Z0-9_]+>=[0-9]+$'
	for req in "$@"; do
		if [[ ! ${req} =~ ${req_re} ]]; then
			echo "gce_region_quota_headroom: malformed requirement '${req}', expected '<METRIC>>=<needed>' (e.g. 'CPUS>=6')" >&2
			return 1
		fi
		metric="${req%%>=*}"
		needed="${req##*>=}"
		# A missing metric means no grant in this region: headroom 0.
		# Compare the raw values without rounding: fractional usage is
		# realistic (shared-core instances report fractional vCPUs).
		headroom=$(awk -v m="${metric}" '$1 == m { printf "%g", $2 - $3; found=1; exit } END { if (!found) print 0 }' <<<"${quotas}")
		if awk -v h="${headroom}" -v n="${needed}" 'BEGIN { exit !(h+0 < n+0) }'; then
			echo "    | region ${region}: quota ${metric} headroom ${headroom} below required ${needed}"
			return 1
		fi
	done
	return 0
}

# gce_log_is_infra_failure reports whether any of the given logs contains an
# unambiguous GCE infrastructure failure signature, as opposed to a genuine
# test failure. Only provider-side conditions are matched:
#   - ZONE_RESOURCE_POOL_EXHAUSTED: the machine type is out of stock.
#   - QUOTA_EXCEEDED: the project has no (or not enough) quota.
#   - machineTypes/<name>' was not found: the type is not offered in the zone.
gce_log_is_infra_failure() {
	grep -E -q -m 1 \
		-e "ZONE_RESOURCE_POOL_EXHAUSTED" \
		-e "QUOTA_EXCEEDED" \
		-e "machineTypes/[a-zA-Z0-9-]+' was not found" \
		"$@" 2>/dev/null
}

# gce_select_region probes GCE_REGION_CANDIDATES in order and exports
# BOOTSTRAP_REGION with the first region that is UP, offers all required
# machine types and has enough spare quota. Regions in GCE_EXCLUDED_REGIONS
# are skipped. Returns 1 when no candidate qualifies.
gce_select_region() {
	: "${GCE_REGION_CANDIDATES:?GCE_REGION_CANDIDATES must list candidate regions in preference order}"

	# An unset requirement variable disables that check; say so, so that a
	# suite forgetting (or mistyping) one does not silently probe nothing.
	if [[ -z ${GCE_REQUIRED_MACHINE_TYPES:-} ]]; then
		echo "    | note: GCE_REQUIRED_MACHINE_TYPES is unset, machine type check disabled"
	fi
	if [[ -z ${GCE_REQUIRED_QUOTAS:-} ]]; then
		echo "    | note: GCE_REQUIRED_QUOTAS is unset, quota check disabled"
	fi

	local region machine_type usable describe status quotas
	# Word splitting of the candidate and machine type lists is intended.
	# shellcheck disable=SC2086
	for region in ${GCE_REGION_CANDIDATES}; do
		if echo " ${GCE_EXCLUDED_REGIONS:-} " | grep -q " ${region} "; then
			echo "    | region ${region}: excluded after a previous infrastructure failure"
			continue
		fi

		usable=0
		# One describe per candidate feeds both the status and the quota
		# checks.
		if ! describe=$(gcloud compute regions describe "${region}" --format=json 2>&1); then
			echo "    | region ${region}: GCE API error describing region: ${describe}"
			usable=1
		fi
		if [[ ${usable} -eq 0 ]]; then
			status=$(yq -r '.status' <<<"${describe}")
			if [[ ${status} != "UP" ]]; then
				echo "    | region ${region}: region is not UP"
				usable=1
			fi
		fi
		for machine_type in ${GCE_REQUIRED_MACHINE_TYPES:-}; do
			if [[ ${usable} -eq 0 ]] && ! gce_machine_type_available "${region}" "${machine_type}"; then
				usable=1
			fi
		done
		if [[ ${usable} -eq 0 && -n ${GCE_REQUIRED_QUOTAS:-} ]]; then
			quotas=$(yq -r '.quotas[]? | [.metric, .limit, .usage] | join(" ")' <<<"${describe}")
			# Word splitting of the requirement list is intended.
			# shellcheck disable=SC2086
			gce_region_quota_headroom "${region}" "${quotas}" ${GCE_REQUIRED_QUOTAS} || usable=1
		fi

		if [[ ${usable} -eq 0 ]]; then
			export BOOTSTRAP_REGION="${region}"
			echo "====> Selected GCE region $(green "${region}")"
			return 0
		fi
	done

	echo "==> ERROR: no usable GCE region for this suite" >&2
	echo "    candidates: ${GCE_REGION_CANDIDATES}" >&2
	echo "    excluded:${GCE_EXCLUDED_REGIONS:- none}" >&2
	echo "    required machine types: ${GCE_REQUIRED_MACHINE_TYPES:-none}" >&2
	echo "    required quotas: ${GCE_REQUIRED_QUOTAS:-none}" >&2
	return 1
}

# gce_run_with_region_fallback bootstraps a controller and runs the given
# command (the suite body) in the first usable GCE region. When the run
# fails for a GCE infrastructure reason (capacity or quota, see
# gce_log_is_infra_failure), the failed region is excluded and the whole
# run is retried in the next usable region, until one succeeds, no
# candidate region remains or GCE_MAX_ATTEMPTS is exhausted, in which case
# the suite fails fast. Genuine test failures never trigger a region
# switch.
#
# ```
# gce_run_with_region_fallback <model name> <bootstrap log file> <command...>
# ```
gce_run_with_region_fallback() {
	local model log
	model=${1}
	log=${2}
	shift 2

	local max_attempts=${GCE_MAX_ATTEMPTS:-3}
	local suite_status attempt=0 f
	while true; do
		attempt=$((attempt + 1))
		if [[ ${attempt} -gt ${max_attempts} ]]; then
			echo "==> $(red 'Failing fast'): ${max_attempts} GCE region attempt(s) exhausted (excluded:${GCE_EXCLUDED_REGIONS:- none})" >&2
			echo "    per-attempt logs: ${log}.attempt-* ${TEST_DIR}/${TEST_CURRENT}.log.attempt-*" >&2
			exit 1
		fi

		if ! gce_select_region; then
			echo "==> $(red 'Failing fast'): no GCE region satisfies the suite requirements" >&2
			if [[ ${attempt} -gt 1 ]]; then
				echo "    previous attempts failed with GCE infrastructure issues; per-attempt logs:" >&2
				echo "    ${log}.attempt-* ${TEST_DIR}/${TEST_CURRENT}.log.attempt-*" >&2
			fi
			exit 1
		fi

		echo "====> Running suite in region $(green "${BOOTSTRAP_REGION}") (excluded:${GCE_EXCLUDED_REGIONS:- none})"

		# The suite body runs in a subshell with strict flags so that any
		# failure aborts it immediately, while this loop survives to decide
		# whether another region should be tried.
		set +e
		(
			set_verbosity
			bootstrap "${model}" "${log}"
			"$@"
			destroy_controller "${model}"
		)
		suite_status=$?
		set_verbosity

		if [[ ${suite_status} -eq 0 ]]; then
			return 0
		fi

		# Classification greps the bootstrap log, the suite summary log and
		# the per-subtest logs: a GCE infrastructure failure during a
		# subtest's add-model lands in the subtest-local file, not in the
		# first two.
		if ! gce_log_is_infra_failure "${log}" "${TEST_DIR}/${TEST_CURRENT}.log" "${TEST_DIR}"/test-*.log; then
			echo "==> Failure is not a GCE infrastructure issue, not trying another region"
			exit 1
		fi

		echo "====> GCE infrastructure failure in $(red "${BOOTSTRAP_REGION}"), excluding it and trying another region"
		GCE_EXCLUDED_REGIONS="${GCE_EXCLUDED_REGIONS:-} ${BOOTSTRAP_REGION}"
		export GCE_EXCLUDED_REGIONS
		# Force the next bootstrap to create a fresh controller instead of
		# reusing the broken one. The leftover controller is destroyed by
		# cleanup_jujus when the suite exits.
		export BOOTSTRAP_REUSE="false"

		# Rotate the logs the failure classification above greps, naming
		# them after the attempt that produced them: OUTPUT appends, so
		# without this a previous attempt's infrastructure signature would
		# misclassify the next attempt's genuine failure.
		for f in "${log}" "${TEST_DIR}/${TEST_CURRENT}.log" "${TEST_DIR}"/test-*.log; do
			if [[ -f ${f} ]]; then
				mv "${f}" "${f}.attempt-${attempt}"
			fi
		done
	done
}
