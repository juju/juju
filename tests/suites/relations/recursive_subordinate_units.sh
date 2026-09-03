# Ensure that subordinate applications related to each other do not create
# subordinate units recursively.

# subordinate_idle_condition emits a yq query matching only when the given
# subordinate unit of the given principal application exists and is idle.
# The built-in idle_subordinate_condition cannot be used here: its select is
# evaluated over an empty stream and also matches before the subordinate
# unit exists.
subordinate_idle_condition() {
	local parent app
	parent=${1}
	app=${2}

	echo "[.applications[\"${parent}\"].units[] | (.subordinates // {}) | to_entries[] | select(.key == \"${app}/0\" and .value[\"juju-status\"].current == \"idle\") | .key] | .[]"
}

# subordinate_unit_count prints the number of units of the given subordinate
# application keyed to the given principal application.
subordinate_unit_count() {
	local parent app
	parent=${1}
	app=${2}

	juju status --format=json | yq "[.applications[\"${parent}\"].units[] | (.subordinates // {}) | to_entries[] | select(.key | test(\"^${app}/\"))] | length"
}

# subordinate_count_stable returns once the number of units of the given
# subordinate application keyed to the principal has been observed to be
# exactly one on two consecutive samples, one SHORT_TIMEOUT apart. Requiring
# a stable window instead of a fixed sleep means neither a busy CI runner
# (more headroom up to SUBORDINATE_SETTLE_TIMEOUT) nor a fast one (exits as
# soon as two matches) is penalised, and a spurious relation-joined hook
# cycle has a bounded window in which to surface an extra unit.
subordinate_count_stable() {
	local subordinate deadline current last
	subordinate=${1}
	deadline=$(( $(date +%s) + ${SUBORDINATE_SETTLE_TIMEOUT:-60} ))
	last=""

	while :; do
		current=$(subordinate_unit_count principal "${subordinate}")
		if [[ ${current} == "1" && ${current} == "${last}" ]]; then
			return 0
		fi
		last=${current}

		if (( $(date +%s) >= deadline )); then
			echo "subordinate count for ${subordinate} did not stabilise at 1 (last=${current})" >&2
			return 1
		fi
		sleep "${SHORT_TIMEOUT}"
	done
}

run_recursive_subordinate_units() {
	echo

	model_name="test-recursive-subordinate-units"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# Deploy the same subordinate charm twice: both applications attach to
	# the principal via juju-info and relate to each other through the
	# container scoped subordinate-link endpoints.
	juju deploy "$(pack_charm "${CURRENT_DIR}"/../testcharms/charms/lxd-profile)" principal
	juju deploy "$(pack_charm "${CURRENT_DIR}"/../testcharms/charms/subordinate-link)" subordinate-one
	juju deploy "$(pack_charm "${CURRENT_DIR}"/../testcharms/charms/subordinate-link)" subordinate-two
	juju integrate subordinate-one principal
	juju integrate subordinate-two principal
	juju integrate subordinate-one:link-a subordinate-two:link-b

	wait_for "subordinate-one/0" "$(subordinate_idle_condition principal subordinate-one)"
	wait_for "subordinate-two/0" "$(subordinate_idle_condition principal subordinate-two)"

	# Subordinate units live under the principal unit's subordinates. Both
	# must be keyed to the principal unit itself: keying one of them to the
	# other subordinate is what causes the recursive deployment. Once the
	# count has stabilised, assert that no extra subordinate unit was spawned
	# during the observation window.
	subordinate_count_stable subordinate-one
	subordinate_count_stable subordinate-two

	check "1" <<<"$(subordinate_unit_count principal subordinate-one)"
	check "1" <<<"$(subordinate_unit_count principal subordinate-two)"

	destroy_model "${model_name}"
}

test_recursive_subordinate_units() {
	if [ "$(skip 'test_recursive_subordinate_units')" ]; then
		echo "==> TEST SKIPPED: recursive subordinate unit tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_recursive_subordinate_units"
	)
}
