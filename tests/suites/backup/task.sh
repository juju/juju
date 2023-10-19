test_backup() {
	echo "==> TEST SKIPPED: Backup and Restore tests"
	return

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju jq

	file="${TEST_DIR}/test-backup-restore.log"

	bootstrap "test-backup" "${file}"

	test_basic_backup

	destroy_controller "test-backup"
}
