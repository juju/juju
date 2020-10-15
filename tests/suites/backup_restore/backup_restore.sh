run_basic_backup_create() {
    set -e  # TODO benhoyt: remove once "set +e" issue is fixed
    echo

    juju switch controller
    juju create-backup --filename "${TEST_DIR}/basic_backup.tar.gz"

    # Do some basic sanity checks on what's inside the backup
    tar xf "${TEST_DIR}/basic_backup.tar.gz" -C "${TEST_DIR}"
    echo "checking metadata.json is present"
    test -s "${TEST_DIR}/juju-backup/metadata.json"
    echo "checking root.tar is present"
    test -s "${TEST_DIR}/juju-backup/root.tar"
    echo "checking oplog.bson is present"
    test -s "${TEST_DIR}/juju-backup/dump/oplog.bson"
}

run_basic_backup_restore() {
    set -e  # TODO benhoyt: remove once "set +e" issue is fixed
    echo

    wget -O "${TEST_DIR}/juju-restore" https://github.com/juju/juju-restore/releases/latest/download/juju-restore
    chmod +x "${TEST_DIR}/juju-restore"

    file="${TEST_DIR}/test-basic-backup-restore.log"

    ensure "test-basic-backup-restore" "${file}"

    echo "Deploy a workload (1 machine)"
    juju deploy cs:~jameinel/ubuntu-lite-7
    wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"
    juju status --format json | jq '.machines | length' | check 1

    echo "Create a backup"
    juju switch controller
    juju create-backup --filename "${TEST_DIR}/basic_backup.tar.gz"

    echo "Add another machine (after the backup)"
    juju switch test-basic-backup-restore
    juju add-unit ubuntu-lite
    juju status --format json | jq '.machines | length' | check 2

    echo "Restore the backup"
    juju switch controller
    juju scp "${TEST_DIR}/juju-restore" 0:
    juju scp "${TEST_DIR}/basic_backup.tar.gz" 0:
    juju ssh 0 ./juju-restore --yes basic_backup.tar.gz

    echo "Ensure there's only one machine (state before the backup)"
    juju switch test-basic-backup-restore
    wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"
    juju status --format json | jq '.machines | length' | check 1

    destroy_model "test-basic-backup-restore"
}

test_basic_backup() {
    if [ "$(skip 'test_basic_backup')" ]; then
        echo "==> TEST SKIPPED: basic backup"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_basic_backup_create"
        run "run_basic_backup_restore"
    )
}