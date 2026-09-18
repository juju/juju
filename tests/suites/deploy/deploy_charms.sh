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
	base_name=$(juju status --format=json | yq -r '.applications."ubuntu-lite".base.name')
	base_chan=$(juju status --format=json | yq -r '.applications."ubuntu-lite".base.channel')
	echo "$base_name@$base_chan" | check "$expected_base"

	destroy_model "test-deploy-charm-placement-directive"
}

run_deploy_charm_zone_placement_directive() {
	echo

	file="${TEST_DIR}/test-deploy-charm-zone-placement-directive.log"

	ensure "test-deploy-charm-zone-placement" "${file}"

	# Add a machine so that we can discover the availability zone for
	# the cloud under test. The zone is extracted from the machine's
	# hardware info and used in the zone-based placement directive below.
	if [[ ${BOOTSTRAP_PROVIDER} == "lxd" ]]; then
		juju add-machine --constraints="virt-type=virtual-machine"
	else
		juju add-machine
	fi
	wait_for_machine_agent_status "0" "started"

	# Extract the availability zone from the machine hardware field.
	# The hardware field is a space-separated string of key=value pairs,
	# e.g. "availability-zone=us-east-1a arch=amd64 cores=4 ..."
	az=$(juju show-machine 0 --format=json |
		yq -r '.["machines"]["0"]["hardware"]' |
		grep -oP 'availability-zone=\K\S+')

	if [[ -z "${az}" ]]; then
		echo "==> TEST SKIPPED: no availability zone reported by provider ${BOOTSTRAP_PROVIDER}"
		destroy_model "test-deploy-charm-zone-placement"
		return
	fi

	# Deploy a local charm using a zone-based placement directive.
	# A local charm deploy goes through the legacy Deploy API path, where
	# the client substitutes the "model-uuid" placeholder scope with the
	# real model UUID before sending it to the API server. The domain
	# layer must recognise the model UUID scope as a provider placement
	# directive, not attempt to parse it as a container type.
	# shellcheck disable=SC2046
	juju deploy $(pack_charm ./testcharms/charms/ubuntu-plus) --to "zone=${az}" ubuntu-zone-placement
	wait_for "ubuntu-zone-placement" "$(idle_condition "ubuntu-zone-placement")"

	# Verify that the machine hosting the deployed unit is in the
	# requested availability zone.
	machine_id=$(juju status --format=json |
		yq -r '.applications."ubuntu-zone-placement".units."ubuntu-zone-placement/0".machine')
	deployed_az=$(juju show-machine "${machine_id}" --format=json |
		yq -r ".[\"machines\"][\"${machine_id}\"][\"hardware\"]" |
		grep -oP 'availability-zone=\K\S+')
	echo "${deployed_az}" | check "${az}"

	destroy_model "test-deploy-charm-zone-placement"
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
	base_name1=$(juju status --format=json | yq -r ".applications.app1.base.name")
	base_chan1=$(juju status --format=json | yq -r ".applications.app1.base.channel")
	base_name2=$(juju status --format=json | yq -r ".applications.app2.base.name")
	base_chan2=$(juju status --format=json | yq -r ".applications.app2.base.channel")

	destroy_model "test-deploy-specific-series"

	echo "$base_name1@$base_chan1" | check "$default_base"
	echo "$base_name2@$base_chan2" | check "$expected_base"
}

run_deploy_local_predeployed_charm() {
	echo

	model_name="test-deploy-local-predeployed-charm"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# shellcheck disable=SC2046
	juju deploy $(pack_charm ./testcharms/charms/ubuntu-plus) --base ubuntu@24.04
	wait_for "ubuntu-plus" "$(idle_condition "ubuntu-plus")"

	juju deploy local:ubuntu-plus-0 another-ubuntu-plus-app
	wait_for "another-ubuntu-plus-app" "$(idle_condition "another-ubuntu-plus-app")"
	wait_for "active" '.applications["another-ubuntu-plus-app"] | ."application-status".current'

	destroy_model "${model_name}"
}

run_deploy_lxd_to_container() {
	# Ensure charms can be deployed to a container and a subordinate
	# charm can be integrated with a principal in a container.
	echo

	model_name="test-deploy-lxd-container"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	charm=$(pack_charm ./testcharms/charms/ubuntu-plus)
	juju deploy ${charm} --to lxd

	# shellcheck disable=SC2046
	juju deploy $(pack_charm ./testcharms/charms/subordinate-link)
	juju integrate subordinate-link ubuntu-plus

	wait_for_container_agent_status "0/lxd/0" "started"
	wait_for "ubuntu-plus" "$(idle_condition "ubuntu-plus")"
	wait_for "subordinate-link/0" '[.applications["ubuntu-plus"].units[] | (.subordinates // {}) | to_entries[] | select(.key == "subordinate-link/0" and .value["juju-status"].current == "idle") | .key] | .[]'

	# The principal unit must be placed on the container, and the
	# subordinate unit must be attached to the principal unit.
	principal_machine=$(juju status --format=json |
		yq -r '.applications["ubuntu-plus"].units["ubuntu-plus/0"].machine')
	echo "${principal_machine}" | check "0/lxd/0"

	# The container's eth0 device is parented to the default LXD bridge,
	# as the suite bootstraps with container-networking-method "local".
	# Assert that eth0 obtained an address, so that a container
	# networking regression fails here deterministically, rather than
	# manifesting only as a deployment timeout.
	eth0_addr="$(juju_exec_output --machine 0/lxd/0 -- ip -4 addr show dev eth0)"
	check_contains "${eth0_addr}" "inet"

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
			if kvm-ok; then
				run "run_deploy_charm_placement_directive"
				run "run_deploy_charm_zone_placement_directive"
			else
				echo "==> TEST SKIPPED: deploy_charm_placement_directive - lxd without kvm is not supported"
				echo "==> TEST SKIPPED: deploy_charm_zone_placement_directive - lxd without kvm is not supported"
			fi
			run "run_deploy_local_predeployed_charm"
			echo "==> TEST SKIPPED: deploy_lxd_to_container - tests for non LXD only"
			;;
		*)
			run "run_deploy_charm_placement_directive"
			run "run_deploy_charm_zone_placement_directive"
			run "run_deploy_lxd_to_container"
			;;
		esac
	)
}
