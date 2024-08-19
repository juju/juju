run_charmhub_download() {
	echo
	name="charmhub-download"

	file="${TEST_DIR}/${name}.log"

	ensure "${name}" "${file}"

	output=$(juju download juju-qa-test --filepath="${TEST_DIR}/juju-qa-test.charm" 2>&1 || true)
	check_contains "${output}" 'Fetching charm "juju-qa-test"'

	juju deploy "${TEST_DIR}/juju-qa-test.charm" juju-qa-test
	wait_for "juju-qa-test" "$(active_idle_condition "juju-qa-test")" 900

	destroy_model "${name}"
}

run_charmhub_download_with_resources() {
	echo
	name="charmhub-download-with-resources"

	file="${TEST_DIR}/${name}.log"

	ensure "${name}" "${file}"

	output=$(juju download juju-qa-test-resources --resources --filepath="${TEST_DIR}/juju-qa-test-resources.charm" 2>&1 || true)
	check_contains "${output}" 'Fetching charm "juju-qa-test-resources"'

	$(echo "${output}" | grep "juju deploy") juju-qa-test-resources
	wait_for "${MODEL_ARCH:-amd64}-linux" "$(workload_status juju-qa-test-resources 0).message"

	destroy_model "${name}"
}

run_charmstore_download() {
	echo
	name="test-charmstore-download"

	file="${TEST_DIR}/${name}.log"

	ensure "${name}" "${file}"

	output=$(juju download cs:meshuggah 2>&1 || echo "not found")
	check_contains "${output}" 'ERROR charm or bundle name, "cs:meshuggah", is not valid'

	destroy_model "${name}"
}

run_unknown_download() {
	echo
	name="test-unknown-download"

	file="${TEST_DIR}/${name}.log"

	ensure "${name}" "${file}"

	output=$(juju download meshuggah 2>&1 || echo "not found")
	check_contains "${output}" "The Charm with the given name was not found in the Store"

	destroy_model "${name}"
}

test_charmhub_download() {
	if [ "$(skip 'test_charmhub_download')" ]; then
		echo "==> TEST SKIPPED: charmhub download"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_charmhub_download"
		run "run_charmhub_download_with_resources"
		run "run_charmstore_download"
		run "run_unknown_download"
	)
}
