run_diff_bundle_reflexive() {
	echo

	file="${TEST_DIR}/test-cli-diff-bundle-reflexive.log"

	ensure "test-cli-diff-bundle-reflexive" "${file}"

	# Check that numbers are considered equal, even if YAML and JSON choose to interpret them differently

	# Apache has integer config options
	juju deploy apache2
	# PostgreSQL has float config options
	# that default to integral values
	juju deploy postgresql

	juju diff-bundle <(juju export-bundle --include-charm-defaults --include-series) | check "{}"

	destroy_model "test-cli-diff-bundle-reflexive"
}

test_diff_bundle() {
	if [ "$(skip 'test_diff_bundle')" ]; then
		echo "==> TEST SKIPPED: diff-bundle tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_diff_bundle_reflexive"
	)
}
