test_deploy_and_remove_application() {
	if [ -n "$(skip 'test_deploy_and_remove_application')" ]; then
		echo "==> SKIP: Asked to skip deploy and remove application"
		return
	fi
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
	juju remove-application snappass-test --no-prompt
	wait_for "0" '.applications | length'

	# Clean up model
	destroy_model "${model_name}"
}

test_deploy_and_force_remove_application() {
	if [ -n "$(skip 'test_deploy_and_force_remove_application')" ]; then
		echo "==> SKIP: Asked to skip deploy and force remove application"
		return
	fi
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
	juju remove-application snappass-test --force --no-prompt
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

test_pebble_notices() {
	if [ -n "$(skip 'test_pebble_notices')" ]; then
		echo "==> SKIP: Asked to skip pebble notices"
		return
	fi
	echo

	# Ensure that a valid Juju controller exists
	model_name="controller-model-sidecar"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"

	# Deploy Pebble Notices test application
	juju deploy juju-qa-pebble-notices
	wait_for "active" '.applications["juju-qa-pebble-notices"].units["juju-qa-pebble-notices/0"]["workload-status"].current'

	# Check that it's responding to new notices
	juju ssh --container redis juju-qa-pebble-notices/0 /charm/bin/pebble notify foo.com/bar key=val
	wait_for "maintenance" '.applications["juju-qa-pebble-notices"].units["juju-qa-pebble-notices/0"]["workload-status"].current'
	wait_for "notice type=custom key=foo.com/bar" '.applications["juju-qa-pebble-notices"].units["juju-qa-pebble-notices/0"]["workload-status"].message'

	juju ssh --container redis juju-qa-pebble-notices/0 /charm/bin/pebble notify example.com/bazz key=val
	wait_for "maintenance" '.applications["juju-qa-pebble-notices"].units["juju-qa-pebble-notices/0"]["workload-status"].current'
	wait_for "notice type=custom key=example.com/bazz" '.applications["juju-qa-pebble-notices"].units["juju-qa-pebble-notices/0"]["workload-status"].message'

	# Clean up model
	destroy_model "${model_name}"
}

test_pebble_checks() {
	if [ -n "$(skip 'test_pebble_checks')" ]; then
		echo "==> SKIP: Asked to skip pebble checks"
		return
	fi
	echo

	# Ensure that a valid Juju controller exists
	model_name="controller-model-sidecar"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"

	# Deploy Pebble checks test application
	juju deploy juju-qa-pebble-checks

	# Check that the charm responds correctly when a check fails, which will be
	# the initial state.
	wait_for "maintenance" '.applications["juju-qa-pebble-checks"].units["juju-qa-pebble-checks/0"]["workload-status"].current'
	wait_for "check failed: exec-check" '.applications["juju-qa-pebble-checks"].units["juju-qa-pebble-checks/0"]["workload-status"].message'

	# Check that the charm responds correctly to the check recovering.
	juju ssh --container ubuntu juju-qa-pebble-checks/0 mkdir /trigger/
	wait_for "active" '.applications["juju-qa-pebble-checks"].units["juju-qa-pebble-checks/0"]["workload-status"].current'
	wait_for "check recovered: exec-check" '.applications["juju-qa-pebble-checks"].units["juju-qa-pebble-checks/0"]["workload-status"].message'

	# Clean up model
	destroy_model "${model_name}"
}
