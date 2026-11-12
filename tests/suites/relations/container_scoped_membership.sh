# Ensure that units on a container scoped relation only see the units they
# are co-located with: each subordinate unit must see only the principal
# unit it belongs to, and on a subordinate to subordinate relation only the
# counterpart unit running on the same principal unit.
#
# Regression test for https://github.com/juju/juju/issues/23742: without
# the fix, each subordinate unit sees every counterpart unit of the
# relation, including those running on other machines.

# subordinate_idle_condition_for emits a yq query matching only when the
# given fully named subordinate unit of the given principal application
# exists and is idle.
subordinate_idle_condition_for() {
	local parent unit
	parent=${1}
	unit=${2}

	echo "[.applications[\"${parent}\"].units[] | (.subordinates // {}) | to_entries[] | select(.key == \"${unit}\" and .value[\"juju-status\"].current == \"idle\") | .key] | .[]"
}

# subordinates_of prints the names of the subordinate units of the given
# principal unit, filtered by subordinate application name prefix.
subordinates_of() {
	local principal_app principal_unit prefix
	principal_app=${1}
	principal_unit=${2}
	prefix=${3}

	juju status --format=json | yq -r ".applications[\"${principal_app}\"].units[\"${principal_unit}\"].subordinates // {} | keys[] | select(test(\"^${prefix}\"))"
}

# relation_list_count prints the number of members relation-list returns
# for the given unit and relation id.
relation_list_count() {
	local unit rel_id
	unit=${1}
	rel_id=${2}

	juju_exec_output --unit "${unit}" "relation-list -r ${rel_id}" 2>/dev/null | grep -c .
}

# relation_list_count_stable prints the relation-list member count once it
# has been observed unchanged, and at least one, over two consecutive
# samples one SHORT_TIMEOUT apart. Waiting for a stable window instead of a
# fixed sleep means neither a busy CI runner (more headroom up to
# MEMBERSHIP_SETTLE_TIMEOUT) nor a fast one (exits as soon as two matches)
# is penalised, and a spurious membership change has a bounded window in
# which to surface.
relation_list_count_stable() {
	local unit rel_id deadline last current
	unit=${1}
	rel_id=${2}

	deadline=$(( $(date +%s) + ${MEMBERSHIP_SETTLE_TIMEOUT:-60} ))
	last=""

	while :; do
		current=$(relation_list_count "${unit}" "${rel_id}")
		if [[ ${current} == "${last}" ]] && (( current >= 1 )); then
			echo "${current}"
			return 0
		fi
		last=${current}

		if (( $(date +%s) >= deadline )); then
			echo "relation-list membership on ${unit} (${rel_id}) did not stabilise (last=${current})" >&2
			return 1
		fi
		sleep "${SHORT_TIMEOUT}"
	done
}

run_container_scoped_membership() {
	echo

	model_name="test-container-scoped-membership"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# Deploy a principal with two units and the same subordinate charm
	# twice. Both subordinates attach to the principal via juju-info and
	# relate to each other through the container scoped link endpoints.
	juju deploy "$(pack_charm "${CURRENT_DIR}"/../testcharms/charms/ubuntu-plus)" principal -n 2
	juju deploy "$(pack_charm "${CURRENT_DIR}"/../testcharms/charms/subordinate-link)" subordinate-one
	juju deploy "$(pack_charm "${CURRENT_DIR}"/../testcharms/charms/subordinate-link)" subordinate-two
	juju integrate subordinate-one principal
	juju integrate subordinate-two principal
	juju integrate subordinate-one:link-a subordinate-two:link-b

	wait_for "subordinate-one/0" "$(subordinate_idle_condition_for principal subordinate-one/0)"
	wait_for "subordinate-one/1" "$(subordinate_idle_condition_for principal subordinate-one/1)"
	wait_for "subordinate-two/0" "$(subordinate_idle_condition_for principal subordinate-two/0)"
	wait_for "subordinate-two/1" "$(subordinate_idle_condition_for principal subordinate-two/1)"

	# Resolve which subordinate unit runs on which principal unit from juju
	# status rather than assuming creation order.
	sub_one_on_p0=$(subordinates_of principal principal/0 subordinate-one)
	sub_one_on_p1=$(subordinates_of principal principal/1 subordinate-one)
	sub_two_on_p0=$(subordinates_of principal principal/0 subordinate-two)
	sub_two_on_p1=$(subordinates_of principal principal/1 subordinate-two)

	# On the juju-info relation, each subordinate unit must see only its
	# own principal unit. Membership is asserted exactly (not via grep):
	# command substitution strips trailing newlines.
	for spec in "${sub_one_on_p0}:principal/0" "${sub_one_on_p1}:principal/1" "${sub_two_on_p0}:principal/0" "${sub_two_on_p1}:principal/1"; do
		unit="${spec%%:*}"
		principal_unit="${spec##*:}"
		rel_id=$(juju_exec_output --unit "${unit}" 'relation-ids juju-info')
		check "^1$" <<<"$(relation_list_count_stable "${unit}" "${rel_id}")"
		listed=$(juju_exec_output --unit "${unit}" "relation-list -r ${rel_id}")
		if [ "${listed}" != "${principal_unit}" ]; then
			echo "FAIL: ${unit} lists '${listed}' instead of its own principal '${principal_unit}'" >&2
			return 1
		fi
	done

	# On the link relation, each subordinate unit must see only the
	# counterpart unit co-located on the same principal unit.
	for spec in "${sub_one_on_p0}:${sub_two_on_p0}:link-a" "${sub_one_on_p1}:${sub_two_on_p1}:link-a" "${sub_two_on_p0}:${sub_one_on_p0}:link-b" "${sub_two_on_p1}:${sub_one_on_p1}:link-b"; do
		unit="${spec%%:*}"
		rest="${spec#*:}"
		expected="${rest%%:*}"
		endpoint="${rest##*:}"
		rel_id=$(juju_exec_output --unit "${unit}" "relation-ids ${endpoint}")
		check "^1$" <<<"$(relation_list_count_stable "${unit}" "${rel_id}")"
		listed=$(juju_exec_output --unit "${unit}" "relation-list -r ${rel_id}")
		if [ "${listed}" != "${expected}" ]; then
			echo "FAIL: ${unit} lists '${listed}' instead of its co-located counterpart '${expected}'" >&2
			return 1
		fi
	done

	# NOTE: no assertion is made from the principal side of the juju-info
	# relations. juju-info is the implicit endpoint on the principal
	# application (ubuntu-plus declares no endpoint for it), and implicit
	# endpoints never fire relation hooks on the implicit side, so
	# relation-list is empty there by design — on every Juju version
	# (internal/worker/uniter/relation/resolver.go skips implicit
	# endpoints). The principal-side co-location filtering is exercised by
	# the domain tests and by the issue's scenario, where both endpoints
	# are explicit (e.g. charmed-etcd's cos-agent).

	destroy_model "${model_name}"
}

test_container_scoped_membership() {
	if [ "$(skip 'test_container_scoped_membership')" ]; then
		echo "==> TEST SKIPPED: container scoped membership tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_container_scoped_membership"
	)
}
