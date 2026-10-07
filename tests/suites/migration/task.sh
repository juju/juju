test_migration() {
	if [ "$(skip 'test_migration')" ]; then
		echo "==> TEST SKIPPED: test_migration tests"
		return
	fi

	set_verbosity

	case "${BOOTSTRAP_PROVIDER}" in
	"lxd"|"ec2")
		;;
	*)
		echo "==> TEST SKIPPED: test_migration test runs on the lxd or ec2 provider"
		return
		;;
	esac

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-migration.log"

	bootstrap "test-migration" "${file}"

	# The juju 3.6 tests gate on the juju 3.6 client being available.
	test_migration_36
	test_migration_36_cmr_offering
	test_migration_36_cmr_consuming
	test_migration_36_cmr_spaces
	test_migration_36_abort
	test_migration_36_users_permissions

	# Same-version migrations: both controllers are built from this branch.
	test_migration_basic
	test_migration_abort
	test_migration_version
	test_migration_saas_common
	test_migration_saas_external
	test_migration_saas_consumer

	destroy_controller "test-migration"
}
