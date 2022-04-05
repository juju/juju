run_deploy_and_remove_application() {
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
	address=$(juju status --format=json | jq -r '.applications["snappass-test"].units["snappass-test/0"].address')
	curl "http://${address}:5000" | grep Snappass

	# Remove application
	juju remove-application snappass-test
	wait_for "0" '.applications | length'

	# Clean up model
	destroy_model "${model_name}"
}
