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
	# other subordinate is what causes the recursive deployment. Allow a
	# further relation-joined/hook cycle to run, then verify that no
	# additional subordinate unit has been spawned.
	sleep 30
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
