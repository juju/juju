run_charmstore_charmrevisionupdater() {
	# Echo out to ensure nice output to the test suite.
	echo

	model_name="test-cs-charmrevisionupdater"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# Deploy an old revision of postgresql
	juju deploy cs:postgresql-238

	# Wait for revision update worker to update the available revision.
	# eg can-upgrade-to: cs:postgresql-239
	wait_for "cs:postgresql-" '.applications["postgresql"] | ."can-upgrade-to"'

	destroy_model "${model_name}"
}

run_charmhub_charmrevisionupdater() {
	# Echo out to ensure nice output to the test suite.
	echo

	model_name="test-ch-charmrevisionupdater"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# Deploy an old revision of ubuntu
	juju deploy ubuntu --channel=stable --revision=18

	# Wait for revision update worker to update the available revision.
	# eg can-upgrade-to: ch:ubuntu-18
	wait_for "ubuntu-" '.applications["ubuntu"] | ."can-upgrade-to"'

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

		run "run_charmhub_charmrevisionupdater"
		run "run_charmstore_charmrevisionupdater"
	)
}
