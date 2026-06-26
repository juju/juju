run_diff_bundle_reflexive() {
	echo

	file="${TEST_DIR}/test-cli-diff-bundle-reflexive.log"

	ensure "test-cli-diff-bundle-reflexive" "${file}"

	# Check that numbers are considered equal, even if YAML and JSON choose to interpret them differently
	# The juju-qa-test charm includes both int and float options
	# so we install it from a bundle and then compare the model to the bundle

	bundle="./test/suites/cli/bundles/juju-qa-test_full-options_bundle.yaml"

	juju deploy bundle
	juju diff-bundle "${bundle}" | check "{}"

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
