test_migration() {
	if [ "$(skip 'test_migration')" ]; then
		echo "==> TEST SKIPPED: test_migration tests"
		return
	fi

	set_verbosity

	# Provision the juju 3.6 client un-captured: sudo's password prompt
	# needs the terminal (see ensure_juju36_client). The gate below only
	# validates.
	ensure_juju36_client || return

	if ! gate_reason=$(mig36_gate 2>&1); then
		echo "==> SKIP: test_migration test: ${gate_reason}"
		return
	fi

	echo "==> Checking for dependencies"
	check_dependencies juju

	case "${BOOTSTRAP_PROVIDER}" in
	"k8s")
		file="${TEST_DIR}/test-migration-k8s.log"

		bootstrap "test-migration-k8s" "${file}"

		# Tests that need to be run are added here.
		test_migration_36_cmr_secrets_consumer
		test_migration_36_cmr_secrets_offerer

		# Takes too long to tear down, so forcibly destroy it
		export KILL_CONTROLLER=true
		destroy_controller "test-migration-k8s"
		;;
	"lxd"|"ec2")
		file="${TEST_DIR}/test-migration.log"

		bootstrap "test-migration" "${file}"

		# Tests that need to be run are added here.
		test_migration_36
		test_migration_36_image_metadata
		test_migration_36_cmr_offering
		test_migration_36_cmr_consuming
		test_migration_36_cmr_spaces
		test_migration_36_abort
		test_migration_36_users_permissions
		test_migration_36_relation_egress_override

		destroy_controller "test-migration"
		;;
	*)
		echo "==> TEST SKIPPED: test_migration test runs on the lxd, ec2 or k8s provider"
		return
		;;
	esac
}
