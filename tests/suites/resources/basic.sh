run_deploy_repo_resource() {
	echo
	name="deploy-repo-resource"

	file="${TEST_DIR}/test-${name}.log"

	ensure "test-${name}" "${file}"

	juju deploy juju-qa-test --channel candidate
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"
	juju config juju-qa-test foo-file=true

	# wait for update-status
	wait_for "resource line one: testing four." "$(workload_status juju-qa-test 0).message"

	destroy_model "test-${name}"
}

run_deploy_local_resource() {
	echo
	name="deploy-local-resource"

	file="${TEST_DIR}/test-${name}.log"

	ensure "test-${name}" "${file}"

	juju deploy juju-qa-test --resource foo-file="./tests/suites/resources/foo-file.txt"
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"
	juju config juju-qa-test foo-file=true

	# wait for update-status
	wait_for "resource line one: did the resource attach?" "$(workload_status juju-qa-test 0).message"

	destroy_model "test-${name}"
}

run_resource_attach() {
	echo
	name="resource-attach"

	file="${TEST_DIR}/test-${name}.log"

	ensure "test-${name}" "${file}"

	juju deploy juju-qa-test
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"
	juju attach-resource juju-qa-test foo-file="./tests/suites/resources/foo-file.txt"

	juju config juju-qa-test foo-file=true
	# wait for config-changed, the charm will update the status
	# to include the contents of foo-file.txt
	wait_for "resource line one: did the resource attach?" "$(workload_status juju-qa-test 0).message"

	destroy_model "test-${name}"
}

run_resource_attach_large() {
	echo
	name="resource-attach-large"

	file="${TEST_DIR}/test-${name}.log"

	ensure "test-${name}" "${file}"

	juju deploy juju-qa-test
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"

	# .txt suffix required for attach.
	FILE=$(mktemp /tmp/resource-XXXXX.txt)
	# Use urandom to add alpha numeric characters with new lines added to the file
	dd if=/dev/urandom bs=1048576 count=100 2>/dev/null | base64 >"${FILE}"
	line=$(head -n 1 "${FILE}")
	juju attach-resource juju-qa-test foo-file="${FILE}"

	juju config juju-qa-test foo-file=true
	# wait for config-changed, the charm will update the status
	# to include the contents of foo-file.txt
	wait_for "resource line one: ${line}" "$(workload_status juju-qa-test 0).message"

	rm "${FILE}"
	destroy_model "test-${name}"
}

test_basic_resources() {
	if [ "$(skip 'test_basic_resources')" ]; then
		echo "==> TEST SKIPPED: Resource basics"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_deploy_local_resource"
		run "run_deploy_repo_resource"

		# these belong in a new file when there is time to update the CI jobs.
		run "run_resource_attach"
		run "run_resource_attach_large"
	)
}
