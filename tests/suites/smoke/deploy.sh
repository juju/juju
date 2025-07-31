# run_local_deploy is responsible for deploying revision 1 of the refresher
# charm to first check that deployment is successful. The second part of this
# test refreshes the charm to revision 2 and verifies that the upgrade hook of
# the charm has been run by checking the status message of the unit for the
# string that the charm outputs during it's upgrade hook.
run_local_deploy() {
	echo

	model_name="test-local-deploy"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju deploy --revision=1 --channel=stable --base ubuntu@20.04 juju-qa-refresher
	wait_for "refresher" "$(idle_condition "refresher")"

	# Refresh is removed, add it back in when we support refresh.
	# juju refresh refresher

	# Wait for the refresh to happen and then wait again.
	wait_for "upgrade hook ran v2" "$(workload_status "refresher" 0)"

	# On microk8s, there's a bug where the application blocks the model teardown
	# TODO: remove the next line once this bug is fixed.
	juju remove-application refresher
	destroy_model "${model_name}"
}

run_charmhub_deploy() {
	echo

	model_name="test-charmhub-deploy"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju deploy "juju-qa-test" --revision 22 --channel stable "juju-qa-test"
	# TODO(nvinuesa): Ideally, we want to deploy the next 2 charms with a
	# placement directive, both to make the test faster and also to test
	# the placement directives. However, until machines are not fully
	# migrated to dqlite, placement directives are broken.
	juju deploy "juju-qa-dummy-source"
	juju deploy "juju-qa-dummy-sink"

	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"
	wait_for "dummy-source" "$(idle_condition "dummy-source")"
	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"
	# TODO(nvinuesa): When relations are finished implementing in 4.0, we
	# should integrate dummy-source with dummy-sink.
	# juju integrate dummy-source dummy-sink
	# juju expose dummy-sink

	# Refresh is removed, add it back in when we support refresh.
	# juju refresh "$charm" --revision 23
	# wait_for "$charm" "$(idle_condition_for_rev "$charm" "23")"

	destroy_model "${model_name}"
}

test_deploy() {
	if [ "$(skip 'test_deploy')" ]; then
		echo "==> TEST SKIPPED: smoke deploy tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		#run "run_local_deploy"
		run "run_charmhub_deploy"
	)
}

idle_condition_for_rev() {
	local name rev app_index unit_index

	name=${1}
	rev=${2}
	app_index=${3:-0}
	unit_index=${4:-0}

	path=".[\"$name\"] | .units | .[\"$name/$unit_index\"]"

	echo ".applications | select(($path | .[\"juju-status\"] | .current == \"idle\") and ($path | .[\"workload-status\"] | .current != \"error\") and (.[\"$name\"] | .[\"charm-rev\"] == $rev)) | keys[$app_index]"
}
