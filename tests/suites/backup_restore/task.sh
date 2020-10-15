test_backup_restore() {
    if [ "$(skip 'test_backup_restore')" ]; then
        echo "==> TEST SKIPPED: Backup and Restore tests"
        return
    fi

    set_verbosity

    echo "==> Checking for dependencies"
    check_dependencies juju

    file="${TEST_DIR}/test-backup-restore.log"

    bootstrap "test-cli" "${file}"

    test_basic_backup

    destroy_controller "test-cli"
}
