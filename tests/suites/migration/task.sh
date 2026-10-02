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

	if ! gate_reason=$(mig36_gate 2>&1); then
		echo "==> SKIP: test_migration test: ${gate_reason}"
		return
	fi

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-migration.log"

	bootstrap "test-migration" "${file}"

	# Tests that need to be run are added here.
	test_migration_36
	test_migration_36_cmr_offering
	test_migration_36_cmr_consuming
	test_migration_36_cmr_spaces
	test_migration_36_abort
	test_migration_36_users_permissions

	destroy_controller "test-migration"
}
