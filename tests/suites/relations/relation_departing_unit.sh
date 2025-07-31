run_relation_departing_unit() {
	echo

	model_name="test-relation-departing-unit"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# the log messages the test looks for do not appear if root
	# log level is INFO.
	juju model-config -m "${model_name}" logging-config="<root>=DEBUG"

	echo "Deploy 2 departer instances"
	# shellcheck disable=SC2046
	juju deploy $(pack_charm ./testcharms/charms/departer) -n 2

	wait_for "departer" "$(idle_condition "departer" 0)"
	wait_for "departer" "$(idle_condition "departer" 1)"

	check_contains "$(juju show-unit departer/0 | yq '.departer/0.relation-info[0].related-units')" 'departer/1'
	check_contains "$(juju show-unit departer/1 | yq '.departer/1.relation-info[0].related-units')" 'departer/0'

	echo "Remove departer/1"
	juju remove-unit departer/1

	attempt=10
	while [[ ${attempt} -gt 0 ]]; do
		attempt=$((attempt - 1))

		echo "Check departer/1 is departing the relation"
		got=$(juju show-unit departer/0 | yq '.departer/0.relation-info[0].related-units' || true)
		if [ "${got}" != null ]; then
			# shellcheck disable=SC2046
			echo $(red "expected departer/1 to be removed from departer/0 related-units")
			if [[ ${attempt} -eq 0 ]]; then
				exit 1
			fi
		fi
		sleep 2
	done

	destroy_model "${model_name}"
}

test_relation_departing_unit() {
	if [ "$(skip 'test_relation_departing_unit')" ]; then
		echo "==> TEST SKIPPED: relation departing unit tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_relation_departing_unit"
	)
}
