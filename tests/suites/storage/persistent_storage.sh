# This function tests charm deployment with a single filesystem storage unit and a single persistent block device
# storage unit.
#  Steps taken to the test:
#       - Deploy dummy-storage charm with a single block storage unit (ebs)
#         and a single filesystem storage unit (rootfs).
#       - Check charm status once the deployment is done.
#           > Application status should be active.
#       - Check charm storage units once the deployment is done.
#           > Total number of storage units should be 2.
#           > Name of storage units should be in align with charm config.
#           > Properties of storage units should be as defined.
#               - Storage Type, Persistent Setting and Pool.
run_persistent_storage() {
	echo

	model_name="persistent-storage"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"
	# dummy-storage is going to be deployed with 1 ebs block storage unit
	# and 1 rootfs filesystem storage unit.
	echo "dummy-storage is going to be deployed with 1 ebs block storage unit and 1 rootfs filesystem storage unit"
	juju deploy -m "${model_name}" juju-qa-dummy-storage dummy-storage \
		--storage single-blk=ebs --storage single-fs=rootfs
	echo "Checking current status of app dummy-storage."
	# wait for current application-status to be active
	wait_for "dummy-storage" "$(active_condition "dummy-storage" 0)"
	# wait for current workload-status to be active
	wait_for "active" "$(workload_status "dummy-storage" 0).current"

	echo "Checking total number of storage unit(s)."
	assert_storage 2 "select(.storage) | .storage | keys | length"
	echo "Checking names of storage unit(s)."
	assert_storage 1 ".storage | keys | map(select(test(\"^single-blk/\"))) | length"
	assert_storage 1 ".storage | keys | map(select(test(\"^single-fs/\"))) | length"
	BLK_ID=$(juju storage --format json | yq -r '.storage | keys | map(select(test("^single-blk/"))) | .[]')
	FS_ID=$(juju storage --format json | yq -r '.storage | keys | map(select(test("^single-fs/"))) | .[]')
	assert_storage "${BLK_ID}" '.storage | keys | .[] | select(test("^single-blk/"))'
	assert_storage "${FS_ID}" '.storage | keys | .[] | select(test("^single-fs/"))'
	echo "Check name and total number of storage unit: PASSED."

	#
	# check type, persistent setting and pool of single block storage unit
	#
	assert_storage "block" ".storage[\"${BLK_ID}\"][\"kind\"]"
	echo "Checking persistent setting of single block storage unit"
	assert_storage true ".storage[\"${BLK_ID}\"][\"persistent\"]"
	assert_storage "${BLK_ID}" '.volumes."0".storage'
	assert_storage "ebs" '.volumes."0".pool'
	assert_storage "attached" '.volumes."0".status.current'
	echo "Check properties of single block device storage unit: PASSED."

	#
	# check type, persistent setting and pool of single filesystem storage unit
	#
	assert_storage "filesystem" ".storage[\"${FS_ID}\"][\"kind\"]"
	echo "Checking persistent setting of single filesystem storage unit"
	assert_storage false ".storage[\"${FS_ID}\"][\"persistent\"]"
	# The sole volume is owned by the block storage instance, not the filesystem.
	assert_storage "${BLK_ID}" '.volumes."0".storage'
	assert_storage "ebs" '.volumes."0".pool'
	assert_storage "attached" '.volumes."0".status.current'
	echo "Check properties of single filesystem storage unit: PASSED."

	# assert charm removal message for single block and filesystem storage
	echo "Remove application, check that storage is also removed"
	removal_msg=$(juju remove-application --no-prompt dummy-storage 2>&1)
	echo "${removal_msg}" | grep -cE "will remove storage ${FS_ID}" | check 1
	echo "${removal_msg}" | grep -cE "will detach storage ${BLK_ID}" | check 1
	#
	# wait until an update of storage status occurred. Due to the asynchronous nature of Juju,
	# the status of storage may change from time to time after a Juju CLI is issued, in this test only
	# the existence of storage unit is the point of interest.
	#
	wait_for "{}" ".applications" # we use this wait_for command as an indicator that the application
	# status has changed and now we can check for the storage status and assert that indeed the
	# single filesystem storage unit has been removed successfully.
	sleep 5 # avoid races, sometimes the storage disappears just after the app
	assert_storage false ".storage | has(\"${FS_ID}\")"

	echo "Checking total number of storage unit(s)."
	assert_storage 1 "select(.storage) | .storage | keys | length"
	echo "Check for existence of single block storage"
	assert_storage true ".storage | has(\"${BLK_ID}\")"
	echo "${BLK_ID} found in storage list."

	echo "Check for existence of ${BLK_ID} persistent storage after remove-application"
	assert_storage "${BLK_ID}" '.volumes."0".storage'
	# storage status
	assert_storage "detached" '.volumes."0".status.current'
	echo "Check status of persistent storage ${BLK_ID} after remove-application: PASSED"

	# Deploy charm with an existing detached storage
	juju deploy -m "${model_name}" juju-qa-dummy-storage dummy-storage \
		--attach-storage "${BLK_ID}"
	echo "Checking current status of app dummy-storage."
	# wait for current application-status to be active
	wait_for "dummy-storage" "$(active_condition "dummy-storage" 0)"
	# wait for current workload-status to be active
	wait_for "active" "$(workload_status "dummy-storage" 1).current"
	assert_storage 1 "select(.storage) | .storage | keys | length"
	echo "Checking existence of single block device storage ${BLK_ID}."
	assert_storage true ".storage | has(\"${BLK_ID}\")"
	# assert persistent setting
	echo "Checking persistent setting of storage unit ${BLK_ID}."
	assert_storage true ".storage[\"${BLK_ID}\"][\"persistent\"]"
	# assert storage status
	echo "Checking the status of storage ${BLK_ID} in volumes."
	assert_storage "attached" '.volumes."0".status.current'

	# remove charm
	juju remove-application --no-prompt dummy-storage
	# wait for application to be removed
	wait_for "{}" ".applications"
	# persistent storage should remain after remove-application
	assert_storage true ".storage | has(\"${BLK_ID}\")"
	# remove storage
	juju remove-storage "${BLK_ID}"
	# wait until an update of storage status occurred. Due to the asynchronous nature of Juju,
	# the status of storage may change time to time after a Juju CLI issued.
	# Checking removal of single block device storage {}.
	# Bug 1708340 https://bugs.launchpad.net/juju/+bug/1708340
	# juju storage --format json output will be empty if no storage exists. So we use juju status --format json
	# and check for the storage section.
	wait_for "{}" ".storage"
	destroy_model "persistent_storage"
}

test_persistent_storage() {
	if [ "$(skip 'test_persistent_storage')" ]; then
		echo "==> TEST SKIPPED: persistent storage tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_persistent_storage"
	)
}
