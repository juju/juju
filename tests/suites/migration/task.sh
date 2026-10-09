test_migration() {
	if [ "$(skip 'test_migration')" ]; then
		echo "==> TEST SKIPPED: test_migration tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	case "${BOOTSTRAP_PROVIDER}" in
	"k8s")
		file="${TEST_DIR}/test-migration-k8s.log"

		bootstrap "test-migration-k8s" "${file}"

		# Tests that need to be run are added here.
		test_migration_36_cmr_secrets_consumer
		test_migration_36_cmr_secrets_offerer

		# Same-version migration between controllers built from this branch.
		test_migration_caas

		# Takes too long to tear down, so forcibly destroy it
		export KILL_CONTROLLER=true
		destroy_controller "test-migration-k8s"
		;;
	"lxd"|"ec2")
		file="${TEST_DIR}/test-migration.log"

		bootstrap "test-migration" "${file}"

		# Tests that need to be run are added here.
		test_migration_36
		test_migration_36_cmr_offering
		test_migration_36_cmr_consuming
		test_migration_36_cmr_spaces
		test_migration_36_abort
		test_migration_36_users_permissions
		test_migration_36_relation_egress_override

		# Same-version migrations between controllers built from this branch.
		test_migration_basic
		test_migration_abort
		test_migration_version
		test_migration_saas_common
		test_migration_saas_external
		test_migration_saas_consumer

		destroy_controller "test-migration"
		;;
	*)
		echo "==> TEST SKIPPED: test_migration test runs on the lxd, ec2 or k8s provider"
		return
		;;
	esac
}
