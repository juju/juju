# NOTE: when making changes, remember that all the tests here need to be able
# to run on amd64 AND arm64.

run_deploy_charm() {
	echo

	file="${TEST_DIR}/test-deploy-charm.log"

	ensure "test-deploy-charm" "${file}"

	juju deploy ubuntu-lite
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"

	destroy_model "test-deploy-charm"
}

run_deploy_charm_placement_directive() {
	echo

	file="${TEST_DIR}/test-deploy-charm-placement-directive.log"

	ensure "test-deploy-charm-placement-directive" "${file}"

	expected_base="ubuntu@20.04"
	# Setup machines for placement based on provider used for test.
	# Container in container doesn't work consistently enough,
	# for test. Use kvm via lxc.
	if [[ ${BOOTSTRAP_PROVIDER} == "lxd" ]]; then
		juju add-machine --base "${expected_base}" --constraints="virt-type=virtual-machine"
	else
		juju add-machine --base "${expected_base}"
	fi

	juju add-machine lxd:0 --base "${expected_base}"

	# If 0/lxd/0 is started, so must machine 0 be.
	wait_for_container_agent_status "0/lxd/0" "started"

	juju deploy ubuntu-lite -n 2 --to 0,0/lxd/0
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"

	# Verify based used to create the machines was used during
	# deploy.
	base_name=$(juju status --format=json | jq -r '.applications."ubuntu-lite".base.name')
	base_chan=$(juju status --format=json | jq -r '.applications."ubuntu-lite".base.channel')
	echo "$base_name@$base_chan" | check "$expected_base"

	destroy_model "test-deploy-charm-placement-directive"
}

run_deploy_charm_unsupported_series() {
	# Test trying to deploy a charmhub charm to an operating system
	# never supported in the specified channel. It should fail.
	echo

	testname="test-deploy-charm-unsupported-series"
	file="${TEST_DIR}/${testname}.log"

	ensure "${testname}" "${file}"

	# The charm in 3.0/stable only supports jammy and only
	# one charm has been released to that channel.
	juju deploy juju-qa-test --channel 3.0/stable --base ubuntu@20.04 | grep -q 'charm or bundle not found for channel' || true

	destroy_model "${testname}"
}

run_deploy_specific_series() {
	echo

	file="${TEST_DIR}/test-deploy-specific-series.log"

	ensure "test-deploy-specific-series" "${file}"

	charm_name="juju-qa-refresher"
	# Have to check against default base, to avoid false positives.
	# These two bases should be different.
	default_base="ubuntu@20.04"
	expected_base="ubuntu@22.04"

	juju deploy "$charm_name" app1
	juju deploy "$charm_name" app2 --base "$expected_base"
	base_name1=$(juju status --format=json | jq -r ".applications.app1.base.name")
	base_chan1=$(juju status --format=json | jq -r ".applications.app1.base.channel")
	base_name2=$(juju status --format=json | jq -r ".applications.app2.base.name")
	base_chan2=$(juju status --format=json | jq -r ".applications.app2.base.channel")

	destroy_model "test-deploy-specific-series"

	echo "$base_name1@$base_chan1" | check "$default_base"
	echo "$base_name2@$base_chan2" | check "$expected_base"
}

run_deploy_lxd_profile_charm() {
	echo

	file="${TEST_DIR}/test-deploy-lxd-profile.log"

	ensure "test-deploy-lxd-profile" "${file}"

	# This charm deploys to Xenial by default, which doesn't
	# always result in a machine which becomes fully deployed
	# with the lxd provider.
	juju deploy juju-qa-lxd-profile-without-devices --base ubuntu@22.04
	wait_for "lxd-profile-without-devices" "$(idle_condition "lxd-profile-without-devices")"

	juju status --format=json | jq '.machines | .["0"] | .["lxd-profiles"] | keys[0]' | check "juju-test-deploy-lxd-profile-lxd-profile"

	destroy_model "test-deploy-lxd-profile"
}

run_deploy_lxd_profile_charm_container() {
	echo

	file="${TEST_DIR}/test-deploy-lxd-profile.log"

	ensure "test-deploy-lxd-profile-container" "${file}"

	# This charm deploys to Xenial by default, which doesn't
	# always result in a machine which becomes fully deployed
	# with the lxd provider.
	juju deploy juju-qa-lxd-profile-without-devices --to lxd --base ubuntu@22.04
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

	# shellcheck disable=SC2046
	juju deploy $(pack_charm ./testcharms/charms/lxd-profile) --base ubuntu@22.04
	wait_for "lxd-profile" "$(idle_condition "lxd-profile")"

	juju deploy local:lxd-profile-0 another-lxd-profile-app
	wait_for "another-lxd-profile-app" "$(idle_condition "another-lxd-profile-app")"
	wait_for "active" '.applications["another-lxd-profile-app"] | ."application-status".current'

	destroy_model "${model_name}"
}

run_deploy_local_lxd_profile_charm() {
	echo

	file="${TEST_DIR}/test-deploy-local-lxd-profile.log"

	ensure "test-deploy-local-lxd-profile" "${file}"

	# shellcheck disable=SC2046
	juju deploy $(pack_charm ./testcharms/charms/lxd-profile)
	# shellcheck disable=SC2046
	juju deploy $(pack_charm ./testcharms/charms/lxd-profile-subordinate)
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

	destroy_model "test-deploy-local-lxd-profile"
}

run_deploy_lxd_to_machine() {
	echo

	model_name="test-deploy-lxd-machine"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju add-machine -n 2 --base ubuntu@22.04

	charm=$(pack_charm ./tests/suites/deploy/charms/lxd-profile-alt)
	juju deploy ${charm} --to 0 --base ubuntu@22.04

	# Test the case where we wait for the machine to start
	# before deploying the unit.
	wait_for_machine_agent_status "1" "started"
	juju add-unit lxd-profile-alt --to 1

	wait_for "lxd-profile-alt" "$(idle_condition "lxd-profile-alt")"

	lxc profile show "juju-test-deploy-lxd-machine-lxd-profile-alt-0" |
		grep -E "linux.kernel_modules: ([a-zA-Z0-9\_,]+)?ip_tables,ip6_tables([a-zA-Z0-9\_,]+)?"

	juju refresh "lxd-profile-alt" --path ${charm}

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
	# Ensure profiles get applied correctly to containers
	# and 1 gets added if a subordinate is added.
	echo

	model_name="test-deploy-lxd-container"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	charm=$(pack_charm ./tests/suites/deploy/charms/lxd-profile-alt)
	juju deploy ${charm} --to lxd

	# shellcheck disable=SC2046
	juju deploy $(pack_charm ./testcharms/charms/lxd-profile-subordinate)
	juju integrate lxd-profile-subordinate lxd-profile-alt

	wait_for "lxd-profile-alt" "$(idle_condition "lxd-profile-alt")"
	wait_for "lxd-profile-subordinate" ".applications | keys[1]"

	machine_0="$(machine_container_path 0 0/lxd/0)"
	wait_for "lxd-profile-subordinate" "${machine_0}"

	lxd_profile_name="juju-test-deploy-lxd-container-lxd-profile-alt"
	lxd_profile_sub_name="juju-test-deploy-lxd-container-lxd-profile-subordinate"

	juju status --format=json | jq "${machine_0}" | check "${lxd_profile_name}"
	juju status --format=json | jq "${machine_0}" | check "${lxd_profile_sub_name}"

	OUT=$(juju exec --machine 0 -- sh -c 'sudo lxc profile show "juju-test-deploy-lxd-container-lxd-profile-alt-0"')
	echo "${OUT}" | grep -E "linux.kernel_modules: ([a-zA-Z0-9\_,]+)?ip_tables,ip6_tables([a-zA-Z0-9\_,]+)?"

	juju refresh "lxd-profile-alt" --path ${charm}

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

	charm=$(pack_charm ./testcharms/charms/simple-resolve)
	juju deploy ${charm}

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

		echo "==> Checking for dependencies"
		check_dependencies charmcraft

		cd .. || exit

		run "run_deploy_charm"
		run "run_deploy_specific_series"
		run "run_resolve_charm"
		run "run_deploy_charm_unsupported_series"

		case "${BOOTSTRAP_PROVIDER:-}" in
		"lxd")
			if stat /dev/kvm; then
				run "run_deploy_charm_placement_directive"
			else
				echo "==> TEST SKIPPED: deploy_charm_placement_directive - lxd without kvm is not supported"
			fi
			run "run_deploy_lxd_to_machine"
			run "run_deploy_lxd_profile_charm"
			run "run_deploy_local_predeployed_charm"
			run "run_deploy_local_lxd_profile_charm"
			echo "==> TEST SKIPPED: deploy_lxd_to_container - tests for non LXD only"
			echo "==> TEST SKIPPED: deploy_lxd_profile_charm_container - tests for non LXD only"
			;;
		*)
			run "run_deploy_charm_placement_directive"
			echo "==> TEST SKIPPED: deploy_lxd_to_machine - tests for LXD only"
			echo "==> TEST SKIPPED: deploy_lxd_profile_charm - tests for LXD only"
			echo "==> TEST SKIPPED: deploy_local_lxd_profile_charm - tests for LXD only"
			run "run_deploy_lxd_to_container"
			run "run_deploy_lxd_profile_charm_container"
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
