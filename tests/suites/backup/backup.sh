run_basic_backup_create() {
	echo

	file="${TEST_DIR}/test-basic-backup-create.log"

	ensure "test-basic-backup-create" "${file}"

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

	destroy_model "test-basic-backup-create"
}

run_basic_backup_restore() {
	echo

	wget -O "${TEST_DIR}/juju-restore" https://github.com/juju/juju-restore/releases/latest/download/juju-restore
	chmod +x "${TEST_DIR}/juju-restore"

	file="${TEST_DIR}/test-basic-backup-restore.log"

	ensure "test-basic-backup-restore" "${file}"

	echo "Deploy a workload (1 machine)"
	juju deploy jameinel-ubuntu-lite
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"
	juju status --format json | jq '.machines | length' | check 1
	id0=$(juju status --format json | jq -r '.machines["0"]["instance-id"]')

	echo "Create a backup"
	juju switch controller # create-backup only works from controller model
	juju create-backup --filename "${TEST_DIR}/basic_backup.tar.gz"

	echo "Add another machine (after the backup)"
	juju switch test-basic-backup-restore
	juju add-unit ubuntu-lite
	wait_for_machine_agent_status "1" "started"
	juju status --format json | jq '.machines | length' | check 2
	id1=$(juju status --format json | jq -r '.machines["1"]["instance-id"]')

	echo "Restore the backup"
	juju switch controller
	juju scp "${TEST_DIR}/juju-restore" 0:
	juju scp "${TEST_DIR}/basic_backup.tar.gz" 0:
	juju ssh 0 ./juju-restore --yes basic_backup.tar.gz

	echo "Ensure there's only one machine (state before the backup)"
	juju switch test-basic-backup-restore
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"
	juju status --format json | jq '.machines | length' | check 1

	# Only do this check if provider is LXD (too hard to do for all providers)
	if [ "${BOOTSTRAP_PROVIDER}" == "lxd" ]; then
		echo "Ensure that both instances are running (restore shouldn't terminate machines)"
		lxc list --format json | jq --arg name "${id0}" -r '.[] | select(.name==$name) | .state.status' | check Running
		lxc list --format json | jq --arg name "${id1}" -r '.[] | select(.name==$name) | .state.status' | check Running
	fi

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
