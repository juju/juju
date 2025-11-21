run_empty_resource_fileobjectstore() {
	echo
	name="empty-resource-fileobjectstore"

	file="${TEST_DIR}/test-${name}.log"

	ensure "test-${name}" "${file}"

	touch "${TEST_DIR}/empty-file.txt"
	juju deploy juju-qa-test qa
	wait_for "qa" "$(idle_condition "qa")"
	juju attach-resource qa foo-file="${TEST_DIR}/empty-file.txt"

	destroy_model "test-${name}"
}

test_empty_resources() {
	if [ "$(skip 'test_empty_resources')" ]; then
		echo "==> TEST SKIPPED: Resource empty"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_empty_resource_fileobjectstore"
	)
}
