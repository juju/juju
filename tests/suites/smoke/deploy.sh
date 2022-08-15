# run_local_deploy is responsible for deploying revision 1 of the refresher
# charm to first check that deployment is successful. The second part of this
# test refreshes the charm to revision 2 and verifies that the upgrade hook of
# the charm has been run by checking the status message of the unit for the
# string that the charm outputs during it's upgrade hook.
run_local_deploy() {
	echo

	file="${2}"

	ensure "test-local-deploy" "${file}"

	juju deploy --revision=1 --channel=stable juju-qa-refresher
	wait_for "refresher" "$(idle_condition "refresher")"

	juju refresh refresher

	# Wait for the refresh to happen and then wait again.
	wait_for "upgrade hook ran v2" "$(workloadstatus "refresher" 0)"

	destroy_model "test-local-deploy"
}

run_charmstore_deploy() {
	echo

	file="${2}"

	ensure "test-charmstore-deploy" "${file}"

	juju deploy cs:~jameinel/ubuntu-lite-6 ubuntu
	wait_for "ubuntu" "$(idle_condition "ubuntu")"

	juju refresh ubuntu
	wait_for "ubuntu" "$(idle_condition_for_rev "ubuntu" "9")"

	destroy_model "test-charmstore-deploy"
}

test_deploy() {
	if [ "$(skip 'test_deploy')" ]; then
		echo "==> TEST SKIPPED: smoke deploy tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		file="${1}"

		# Check that deploy runs on LXD§
		run "run_local_deploy" "${file}"
		run "run_charmstore_deploy" "${file}"
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
