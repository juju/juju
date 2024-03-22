test_deploy_and_remove_application() {
	echo

	# Ensure that a valid Juju controller exists
	model_name="controller-model-sidecar"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"

	# Deploy snappass-test application
	juju deploy snappass-test
	wait_for "active" '.applications["snappass-test"]["application-status"].current'
	wait_for "active" '.applications["snappass-test"].units["snappass-test/0"]["workload-status"].current'

	# Check that it's properly responding
	check_snappass

	# Remove application
	juju remove-application snappass-test
	wait_for "0" '.applications | length'

	# Clean up model
	destroy_model "${model_name}"
}

test_deploy_and_force_remove_application() {
	echo

	# Ensure that a valid Juju controller exists
	model_name="controller-model-sidecar"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"

	# Deploy snappass-test application
	juju deploy snappass-test
	wait_for "active" '.applications["snappass-test"]["application-status"].current'
	wait_for "active" '.applications["snappass-test"].units["snappass-test/0"]["workload-status"].current'

	# Check that it's properly responding
	check_snappass

	# Remove application with --force
	juju remove-application snappass-test --force
	wait_for "0" '.applications | length'

	# Clean up model
	destroy_model "${model_name}"
}

# Check that snappass-test is properly responding
# Allow multiple attempts, as it could fail initially if we try to connect
# before it's ready
check_snappass() {
	attempt=1
	while true; do
		address=$(juju status --format=json | jq -r '.applications["snappass-test"].units["snappass-test/0"].address')
		if curl "http://${address}:5000" | grep Snappass; then
			break
		fi
		if [[ ${attempt} -ge 3 ]]; then
			echo "Failed to connect to application"
			exit 1
		fi
		attempt=$((attempt + 1))
		sleep 5
	done
}

# This is a stub test, as pebble notices support were added only at 3.4 branch and above.
# But we need to keep the test here to avoid breaking the test suite in jenkins.
test_pebble_notices() {
	echo "==> TEST SKIPPED: pebble notices"
}
