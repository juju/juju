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
  OUT=$(juju status -m controller --format=json | jq '.applications.controller.units["controller/0"]."open-ports".[]')
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

		run "run_empty_model_status"
		run "run_controller_ports"
	)

}
