run_charmrevisionupdater() {
	# Echo out to ensure nice output to the test suite.
	echo

	model_name="test-charmrevisionupdater"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# Deploy an old revision of mysql
	juju deploy cs:mysql-55

	# Wait for revision update worker to update the available revision.
	# eg can-upgrade-to: cs:mysql-58
	wait_for "cs:mysql-" '.applications["mysql"] | ."can-upgrade-to"'

	destroy_model "${model_name}"
}

test_charmrevisionupdater() {
	if [ -n "$(skip 'test_charmrevisionupdater')" ]; then
		echo "==> SKIP: Asked to skip charmrevisionupdater tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_charmrevisionupdater"
	)
}
