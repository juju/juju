run_diff_bundle_reflexive() {
	echo

	file="${TEST_DIR}/test-cli-diff-bundle-reflexive.log"

	ensure "test-cli-diff-bundle-reflexive" "${file}"

	# Check that numbers are considered equal, even if YAML and JSON choose to interpret them differently
	# The juju-qa-test charm includes both int and float options
	# so we install it and then compare the model to an exported bundle

	# Pack the local juju-qa-test charm, which includes a float config
	# option that is not yet published to Charmhub.
	charm_dir="./testcharms/charm-hub/charms/juju-qa-test"
	charm_file="${TEST_DIR}/juju-qa-test.charm"
	(
		cd "${charm_dir}" || exit 1
		charmcraft clean
		charmcraft pack --use-lxd
		cp juju-qa-test_*.charm "${charm_file}"
	)

	# Deploy the charm with int and float config options set to
	# integral values. This is the case that triggers the YAML/JSON
	# unmarshaling difference that diff-bundle must handle.
	# A config file is used because --config does not support float values.
	resource_file="$(pwd)/tests/suites/resources/foo-file.txt"
	config_file="${TEST_DIR}/juju-qa-test-config.yaml"
	cat >"${config_file}" <<'EOF'
juju-qa-test:
  skill: 0
  floatiness: 0
EOF
	juju deploy "${charm_file}" juju-qa-test \
		--base ubuntu@22.04 \
		--resource foo-file="${resource_file}" \
		--config "${config_file}"

	# Wait for the application to be active before exporting the bundle.
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"

	# Export the bundle from the model and diff it against itself.
	# A reflexive diff should produce no differences.
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
