test_migration_k8s() {
	if [ "$(skip 'test_migration_k8s')" ]; then
		echo "==> TEST SKIPPED: test_migration_k8s tests"
		return
	fi

	set_verbosity

	if [[ ${BOOTSTRAP_PROVIDER} != "k8s" ]]; then
		echo "==> TEST SKIPPED: test_migration_k8s test runs on k8s only"
		return
	fi

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-migration-k8s.log"

	bootstrap "test-migration-k8s" "${file}"

	# Tests that need to be run are added here.
	test_migration_36_cmr_secrets_consumer
	test_migration_36_cmr_secrets_offerer
	test_migration_caas

	# Takes too long to tear down, so forcibly destroy it
	export KILL_CONTROLLER=true
	destroy_controller "test-migration-k8s"
}
