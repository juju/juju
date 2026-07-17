run_status_filters() {
	echo

	file="${TEST_DIR}/test-status-filters.log"
	ensure "status-filters" "${file}"

	# Deploy two applications with two units each.
	juju deploy ubuntu-lite a -n 2
	juju deploy ubuntu-lite b -n 2
	wait_for "a" "$(idle_condition "a" 0)"
	wait_for "a" "$(idle_condition "a" 1)"
	wait_for "b" "$(idle_condition "b" 0)"
	wait_for "b" "$(idle_condition "b" 1)"

	# No filter: both applications and all four units must appear.
	OUT=$(juju status --format yaml 2>/dev/null)
	echo "${OUT}" | yq '.applications | has("a")' | check "true"
	echo "${OUT}" | yq '.applications | has("b")' | check "true"
	echo "${OUT}" | yq '.applications.a.units | has("a/0")' | check "true"
	echo "${OUT}" | yq '.applications.a.units | has("a/1")' | check "true"
	echo "${OUT}" | yq '.applications.b.units | has("b/0")' | check "true"
	echo "${OUT}" | yq '.applications.b.units | has("b/1")' | check "true"

	# Determine which unit of application a holds leadership.
	a_leader=$(echo "${OUT}" | yq '.applications.a.units | to_entries[] | select(.value.leader == true) | .key')
	echo "Leader of a: ${a_leader}"

	# Filter by application name: only app a and its units should appear.
	OUT=$(juju status --format yaml a 2>/dev/null)
	echo "${OUT}" | yq '.applications | has("a")' | check "true"
	check_not_contains "$(echo "${OUT}" | yq '.applications | keys | .[]')" "b"

	# Filter by unit a/1: only a/1 should appear under application a.
	OUT=$(juju status --format yaml a/1 2>/dev/null)
	echo "${OUT}" | yq '.applications.a.units | has("a/1")' | check "true"
	check_not_contains "$(echo "${OUT}" | yq '.applications.a.units | keys | .[]')" "a/0"
	check_not_contains "$(echo "${OUT}" | yq '.applications | keys | .[]')" "b"

	# Filter by a/leader: only the leader unit should appear.
	OUT=$(juju status --format yaml a/leader 2>/dev/null)
	echo "${OUT}" | yq '.applications.a.units | keys | .[]' | check "${a_leader}"
	check_not_contains "$(echo "${OUT}" | yq '.applications | keys | .[]')" "b"

	# Filter by machine 0: machine 0 and its unit should appear; no other
	# machines should be present.
	OUT=$(juju status --format yaml 0 2>/dev/null)
	echo "${OUT}" | yq '.machines | has("0")' | check "true"
	check_not_contains "$(echo "${OUT}" | yq '.machines | keys | .[]')" "\"1\""
	check_not_contains "$(echo "${OUT}" | yq '.machines | keys | .[]')" "\"2\""
	check_not_contains "$(echo "${OUT}" | yq '.machines | keys | .[]')" "\"3\""

	destroy_model "status-filters"
}

# Tests that juju status for empty models is consistent.
# There should be an empty space between the model status and the error text below it.
run_empty_model_status() {
	echo

	file="${TEST_DIR}/test-empty-model-status.log"
	ensure "empty-model-status" "${file}"

	echo "Print out juju status for empty model"
	status=$(juju status 2>&1)
	# check that the 4th line matches the expected output.
	echo "${status}" | sed -sn 4p | check 'Model "admin/empty-model-status" is empty.'
	# check that the 3rd line is exactly one empty line.
	echo "${status}" | sed -sn 3p | grep -c '^$' | check 1

	destroy_model "empty-model-status"
}

run_controller_ports() {
	echo

	## Check open ports
	OUT=$(juju status -m controller --format=json | yq '.applications.controller.units["controller/0"]."open-ports".[]')
	check_contains "$OUT" "17070/tcp"
	check_contains "$OUT" "17022/tcp"

	juju status -m controller | grep "controller/0" | awk '{print $6}' | check "17022,17070/tcp"
}

test_model_status() {
	if [ -n "$(skip 'test_model_status')" ]; then
		echo "==> SKIP: Asked to skip model status tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_status_filters"
		run "run_empty_model_status"
		run "run_controller_ports"
	)

}
