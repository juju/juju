run_charmhub_download() {
	echo
	name="charmhub-download"

	file="${TEST_DIR}/test-${name}.log"

	ensure "test-${name}" "${file}"

	output=$(juju download postgresql --series focal --filepath="${TEST_DIR}/postgresql.charm" 2>&1 || true)
	check_contains "${output}" 'Fetching charm "postgresql"'

	juju deploy "${TEST_DIR}/postgresql.charm" postgresql
	juju wait-for application --timeout=15m postgresql
}

run_charmstore_download() {
	echo
	name="charmstore-download"

	file="${TEST_DIR}/test-${name}.log"

	ensure "test-${name}" "${file}"

	output=$(juju download cs:meshuggah 2>&1 || echo "not found")
	check_contains "${output}" '"cs:meshuggah" is not a Charmhub charm'
}

run_unknown_download() {
	echo
	name="unknown-download"

	file="${TEST_DIR}/test-${name}.log"

	ensure "test-${name}" "${file}"

	output=$(juju download meshuggah 2>&1 || echo "not found")
	check_contains "${output}" "The Charm with the given name was not found in the Store"
}

test_charmhub_download() {
	if [ "$(skip 'test_charmhub_download')" ]; then
		echo "==> TEST SKIPPED: Charmhub download"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_charmhub_download"
		run "run_charmstore_download"
		run "run_unknown_download"
	)
}
