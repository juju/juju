test_deploy_manual() {
	if [ "$(skip 'test_deploy_manual')" ]; then
		echo "==> TEST SKIPPED: deploy manual"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		case "${BOOTSTRAP_PROVIDER:-}" in
		"lxd")
			run "run_deploy_manual_lxd"
			;;
		"ec2")
			run "run_deploy_manual_aws"
			;;
		*)
			echo "==> TEST SKIPPED: deploy manual - tests for LXD and AWS only"
			;;
		esac
	)
}

manual_deploy() {
	local cloud_name name addr_m1 addr_m2

	cloud_name=${1}
	name=${2}
	addr_m1=${3}
	addr_m2=${4}

	juju add-cloud --client "${cloud_name}" "${TEST_DIR}/cloud_name.yaml" 2>&1 | tee "${TEST_DIR}/add-cloud.log"

	file="${TEST_DIR}/test-${name}.log"

	export BOOTSTRAP_PROVIDER="manual"
	unset BOOTSTRAP_REGION
	bootstrap "${cloud_name}" "test-${name}" "${file}"
	juju switch controller

	juju add-machine ssh:ubuntu@"${addr_m1}" 2>&1 | tee "${TEST_DIR}/add-machine-1.log"
	juju add-machine ssh:ubuntu@"${addr_m2}" 2>&1 | tee "${TEST_DIR}/add-machine-2.log"

	juju add-unit -m controller controller -n 2 --to "1,2" 2>&1 | tee "${TEST_DIR}/enable-ha.log"
	wait_for "controller" "$(active_condition "controller" 0)"

	machine_base=$(juju machines --format=json | jq -r '.machines | .["0"] | (.base.name+"@"+.base.channel)')

	if [[ -z ${machine_base} ]]; then
		echo "machine 0 has invalid base"
		exit 1
	fi

	juju deploy ubuntu --to=0 --base="${machine_base}"

	wait_for "ubuntu" "$(idle_condition "ubuntu" 1)"

	juju remove-application ubuntu

	destroy_controller "test-${name}"

	delete_user_profile "${name}"
}
