run_deploy_charm() {
	echo

	file="${TEST_DIR}/test-deploy-charm.log"

	ensure "test-deploy-charm" "${file}"

	juju deploy jameinel-ubuntu-lite
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"

	destroy_model "test-deploy-charm"
}

run_deploy_specific_series() {
	echo

	file="${TEST_DIR}/test-deploy-specific-series.log"

	ensure "test-deploy-specific-series" "${file}"

	juju deploy postgresql --base ubuntu@20.04
	base_name=$(juju status --format=json | jq ".applications.postgresql.base.name")
	base_channel=$(juju status --format=json | jq ".applications.postgresql.base.channel")

	destroy_model "test-deploy-specific-series"

	echo "$base_name" | check "ubuntu"
	echo "$base_channel" | check "20.04"
}

run_deploy_lxd_profile_charm() {
	echo

	file="${TEST_DIR}/test-deploy-lxd-profile.log"

	ensure "test-deploy-lxd-profile" "${file}"

	juju deploy juju-qa-lxd-profile-without-devices
	wait_for "lxd-profile-without-devices" "$(idle_condition "lxd-profile-without-devices")"

	juju status --format=json | jq '.machines | .["0"] | .["lxd-profiles"] | keys[0]' | check "juju-test-deploy-lxd-profile-lxd-profile"

	destroy_model "test-deploy-lxd-profile"
}

run_deploy_lxd_profile_charm_container() {
	echo

	file="${TEST_DIR}/test-deploy-lxd-profile.log"

	ensure "test-deploy-lxd-profile-container" "${file}"

	juju deploy juju-qa-lxd-profile-without-devices --to lxd
	wait_for "lxd-profile-without-devices" "$(idle_condition "lxd-profile-without-devices")"

	juju status --format=json | jq '.machines | .["0"] | .containers | .["0/lxd/0"] | .["lxd-profiles"] | keys[0]' |
		check "juju-test-deploy-lxd-profile-container-lxd-profile"

	destroy_model "test-deploy-lxd-profile-container"
}

run_deploy_local_predeployed_charm() {
	echo

	model_name="test-deploy-local-predeployed-charm"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju deploy ./testcharms/charms/lxd-profile --base ubuntu@22.04
	wait_for "lxd-profile" "$(idle_condition "lxd-profile")"

	juju deploy local:jammy/lxd-profile-0 another-lxd-profile-app
	wait_for "another-lxd-profile-app" "$(idle_condition "another-lxd-profile-app")"
	wait_for "active" '.applications["another-lxd-profile-app"] | ."application-status".current'

	destroy_model "${model_name}"
}

run_deploy_local_lxd_profile_charm() {
	echo

	file="${TEST_DIR}/test-deploy-local-lxd-profile.log"

	ensure "test-deploy-local-lxd-profile" "${file}"

	juju deploy ./testcharms/charms/lxd-profile
	juju deploy ./testcharms/charms/lxd-profile-subordinate
	juju integrate lxd-profile-subordinate lxd-profile

	wait_for "lxd-profile" "$(idle_condition "lxd-profile")"
	wait_for "lxd-profile-subordinate" ".applications | keys[1]"

	lxd_profile_name="juju-test-deploy-local-lxd-profile-lxd-profile"
	lxd_profile_sub_name="juju-test-deploy-local-lxd-profile-lxd-profile-subordinate"

	# subordinates take longer to show, so use wait_for
	machine_0="$(machine_path 0)"
	wait_for "${lxd_profile_sub_name}" "${machine_0}"

	juju status --format=json | jq "${machine_0}" | check "${lxd_profile_name}"
	juju status --format=json | jq "${machine_0}" | check "${lxd_profile_sub_name}"

	juju add-unit "lxd-profile"

	machine_1="$(machine_path 1)"
	wait_for "${lxd_profile_sub_name}" "${machine_1}"

	juju status --format=json | jq "${machine_1}" | check "${lxd_profile_name}"
	juju status --format=json | jq "${machine_1}" | check "${lxd_profile_sub_name}"

	juju add-unit "lxd-profile" --to lxd

	machine_2="$(machine_container_path 2 2/lxd/0)"
	wait_for "${lxd_profile_sub_name}" "${machine_2}"

	juju status --format=json | jq "${machine_2}" | check "${lxd_profile_name}"
	juju status --format=json | jq "${machine_2}" | check "${lxd_profile_sub_name}"

	destroy_model "test-deploy-local-lxd-profile"
}

run_deploy_lxd_to_machine() {
	echo

	model_name="test-deploy-lxd-machine"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju add-machine -n 2 --series=jammy

	charm=./tests/suites/deploy/charms/lxd-profile-alt
	juju deploy "${charm}" --to 0 --base ubuntu@22.04

	# Test the case where we wait for the machine to start
	# before deploying the unit.
	wait_for_machine_agent_status "1" "started"
	juju add-unit lxd-profile-alt --to 1

	wait_for "lxd-profile-alt" "$(idle_condition "lxd-profile-alt")"

	lxc profile show "juju-test-deploy-lxd-machine-lxd-profile-alt-0" |
		grep -E "linux.kernel_modules: ([a-zA-Z0-9\_,]+)?ip_tables,ip6_tables([a-zA-Z0-9\_,]+)?"

	juju refresh "lxd-profile-alt" --path "${charm}"

	# Ensure that an upgrade will be kicked off. This doesn't mean an upgrade
	# has finished though, just started.
	wait_for "lxd-profile-alt" "$(charm_rev "lxd-profile-alt" 1)"
	wait_for "lxd-profile-alt" "$(idle_condition "lxd-profile-alt")"

	attempt=0
	while true; do
		OUT=$(lxc profile show "juju-test-deploy-lxd-machine-lxd-profile-alt-1" | grep -E "linux.kernel_modules: ([a-zA-Z0-9\_,]+)?ip_tables,ip6_tables([a-zA-Z0-9\_,]+)?" || echo 'NOT FOUND')
		if [ "${OUT}" != "NOT FOUND" ]; then
			break
		fi
		lxc profile show "juju-test-deploy-lxd-machine-lxd-profile-alt-1"
		attempt=$((attempt + 1))
		if [ $attempt -eq 10 ]; then
			# shellcheck disable=SC2046
			echo $(red "timeout: waiting for lxc profile to show 50sec")
			exit 5
		fi
		sleep 5
	done

	# Ensure that the old one is removed
	attempt=0
	while true; do
		OUT=$(lxc profile show "juju-test-deploy-lxd-machine-lxd-profile-alt-0" || echo 'NOT FOUND')
		if [[ ${OUT} == "NOT FOUND" ]]; then
			break
		fi
		attempt=$((attempt + 1))
		if [ $attempt -eq 10 ]; then
			# shellcheck disable=SC2046
			echo $(red "timeout: waiting for removal of lxc profile 50sec")
			exit 5
		fi
		sleep 5
	done

	destroy_model "${model_name}"
}

run_deploy_lxd_to_container() {
	echo

	model_name="test-deploy-lxd-container"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	charm=./tests/suites/deploy/charms/lxd-profile-alt
	juju deploy "${charm}" --to lxd

	wait_for "lxd-profile-alt" "$(idle_condition "lxd-profile-alt")"

	OUT=$(juju exec --machine 0 -- sh -c 'sudo lxc profile show "juju-test-deploy-lxd-container-lxd-profile-alt-0"')
	echo "${OUT}" | grep -E "linux.kernel_modules: ([a-zA-Z0-9\_,]+)?ip_tables,ip6_tables([a-zA-Z0-9\_,]+)?"

	juju refresh "lxd-profile-alt" --path "${charm}"

	# Ensure that an upgrade will be kicked off. This doesn't mean an upgrade
	# has finished though, just started.
	wait_for "lxd-profile-alt" "$(charm_rev "lxd-profile-alt" 1)"
	wait_for "lxd-profile-alt" "$(idle_condition "lxd-profile-alt")"

	attempt=0
	while true; do
		OUT=$(juju exec --machine 0 -- sh -c 'sudo lxc profile show "juju-test-deploy-lxd-container-lxd-profile-alt-1"' || echo 'NOT FOUND')
		if echo "${OUT}" | grep -E -q "linux.kernel_modules: ([a-zA-Z0-9\_,]+)?ip_tables,ip6_tables([a-zA-Z0-9\_,]+)?"; then
			break
		fi
		attempt=$((attempt + 1))
		if [ $attempt -eq 10 ]; then
			# shellcheck disable=SC2046
			echo $(red "timeout: waiting for lxc profile to show 50sec")
			exit 5
		fi
		sleep 5
	done

	# Ensure that the old one is removed
	attempt=0
	while true; do
		OUT=$(juju exec --machine 0 -- sh -c "sudo lxc profile list" || echo 'NOT FOUND')
		if echo "${OUT}" | grep -v "juju-test-deploy-lxd-container-lxd-profile-alt-0"; then
			break
		fi
		attempt=$((attempt + 1))
		if [ $attempt -eq 10 ]; then
			# shellcheck disable=SC2046
			echo $(red "timeout: waiting for removal of lxc profile 50sec")
			exit 5
		fi
		sleep 5
	done

	destroy_model "${model_name}"
}

# Checks the install hook resolving with --no-retry flag
run_resolve_charm() {
	echo

	model_name="test-resolve-charm"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	charm=./testcharms/charms/simple-resolve
	juju deploy "${charm}"

	wait_for "error" '.applications["simple-resolve"] | ."application-status".current'

	juju resolve --no-retry simple-resolve/0

	wait_for "No install hook" '.applications["simple-resolve"] | ."application-status".message'
	wait_for "active" '.applications["simple-resolve"] | ."application-status".current'

	destroy_model "${model_name}"
}

test_deploy_charms() {
	if [ "$(skip 'test_deploy_charms')" ]; then
		echo "==> TEST SKIPPED: deploy charms"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_deploy_charm"
		run "run_deploy_specific_series"
		run "run_deploy_lxd_to_container"
		run "run_deploy_lxd_profile_charm_container"
		run "run_resolve_charm"

		case "${BOOTSTRAP_PROVIDER:-}" in
		"lxd" | "localhost")
			run "run_deploy_lxd_to_machine"
			run "run_deploy_lxd_profile_charm"
			run "run_deploy_local_predeployed_charm"
			run "run_deploy_local_lxd_profile_charm"
			;;
		*)
			echo "==> TEST SKIPPED: deploy_lxd_to_machine - tests for LXD only"
			echo "==> TEST SKIPPED: deploy_lxd_profile_charm - tests for LXD only"
			echo "==> TEST SKIPPED: deploy_local_lxd_profile_charm - tests for LXD only"
			;;
		esac
	)
}

machine_path() {
	local machine

	machine=${1}

	echo ".machines | .[\"${machine}\"] | .[\"lxd-profiles\"] | keys"
}

machine_container_path() {
	local machine container

	machine=${1}
	container=${2}

	echo ".machines | .[\"${machine}\"] | .containers | .[\"${container}\"] | .[\"lxd-profiles\"] | keys"
}
