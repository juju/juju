run_controller_limit_access_in_ha() {
	case "${BOOTSTRAP_PROVIDER:-}" in
	"ec2" | "gce")
		machine_info="$(juju list-machines -m controller --format=json)"
		instance_id="$(yq -r '.machines["0"]."instance-id"' <<<"$machine_info")"
		region_or_az=$(region_or_availability_zone)
		network_tag_or_group=$(instance_network_tag_or_group)

		echo "Limit access to all controllers in HA"
		juju expose -m controller controller --to-cidrs 10.0.0.0/24
		# In HA the firewaller restricts port 17070 on each controller
		# machine sequentially via separate provider firewall calls, so
		# juju status keeps succeeding until all machines are restricted.
		# Use more iterations than the single-controller default (10) to
		# allow for this propagation.
		wait_for_or_fail "! timeout 5 juju status" 30

		echo "Temporarily grant this machine access to the 1st controller in the HA"
		allow_access_to_api_port "${instance_id}" "${region_or_az}" "${network_tag_or_group}"
		wait_for_or_fail "timeout 5 juju status" 30

		echo "Allow access to all controller in HA from anywhere"
		juju expose -m controller controller --to-cidrs 0.0.0.0/0

		# Juju should be able to dump status after removing the temporary network tag
		# to avoid affecting subsequent tests.
		remove_access_to_api_port "${instance_id}" "${region_or_az}" "${network_tag_or_group}"
		wait_for_or_fail "timeout 5 juju status" 30
		;;
	*)
		echo "==> TEST SKIPPED: run_controller_limit_access_in_ha test runs on aws/gce only"
		;;
	esac
}

run_enable_ha() {
	echo

	file="${TEST_DIR}/enable_ha.log"

	ensure "enable-ha" "${file}"

	juju deploy jameinel-ubuntu-lite

	juju enable-ha

	wait_for_controller_machines 3
	wait_for_ha 3

	# Ensure all the controller units are fully deployed before testing the
	# controller in HA mode.
	juju switch controller
	wait_for "controller" "$(idle_condition "controller" 0 0)"
	wait_for "controller" "$(idle_condition "controller" 0 1)"
	wait_for "controller" "$(idle_condition "controller" 0 2)"

	# Run limit api port access
	run_controller_limit_access_in_ha

	juju switch enable-ha
	destroy_model "enable-ha"
}

test_enable_ha() {
	if [ -n "$(skip 'test_enable_ha')" ]; then
		echo "==> SKIP: Asked to skip controller enable-ha tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_enable_ha"
	)
}
