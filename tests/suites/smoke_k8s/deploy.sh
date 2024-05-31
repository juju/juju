run_charmstore_deploy() {
	echo

	file="${2}"

	ensure "test-charmstore-deploy" "${file}"

	juju deploy snappass-test --revision 8 --channel stable
	wait_for "snappass-test" "$(idle_condition "snappass-test")"

	juju refresh snappass-test
	wait_for "snappass-test" "$(idle_condition_for_rev "snappass-test" "9")"

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
