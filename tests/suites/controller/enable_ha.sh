wait_for_controller_no_leader() {
	# We need to wait for the Dqlite cluster to be broken (loss of quorum),
	# before we start waiting for the backstop behaviour to be pending
	# (see wait_for_controller_leader below).
	# shellcheck disable=SC2143
<<<<<<< HEAD
	until ! [[ "$(juju exec -m controller --unit controller/leader uptime | grep load)" ]]; do
		echo "[+] waiting for no controller leadership"
	done
=======
	until [[ "$(juju machines -m controller --format=json | yq -r '.machines | .[] | .["juju-status"] | select(.current == "started") | .current' | wc -l | grep "${amount}")" ]]; do
		echo "[+] (attempt ${attempt}) polling started machines during ha tear down"
		juju machines -m controller 2>&1 | sed 's/^/    | /g' || true
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))

		if [[ ${attempt} -gt 25 ]]; then
			echo "enable-ha failed waiting for only 1 started machine"
			exit 1
		fi
	done

	attempt=0
	# shellcheck disable=SC2143
	until [[ "$(juju machines -m controller --format=json | yq -r '.machines | .[] | .["juju-status"] | select(.current == "stopped") | .current' | wc -l | grep 0)" ]]; do
		echo "[+] (attempt ${attempt}) polling stopped machines during ha tear down"
		juju machines -m controller 2>&1 | sed 's/^/    | /g' || true
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))

		if [[ ${attempt} -gt 25 ]]; then
			echo "enable-ha failed waiting for machines to tear down"
			exit 1
		fi
	done

	if [[ "$(juju machines -m controller --format=json | yq -r '.machines | .[] | .["juju-status"] | select(.current == "error") | .current' | wc -l)" -gt 0 ]]; then
		echo "machine in controller model with error during ha tear down"
		juju machines -m controller 2>&1 | sed 's/^/    | /g' || true
		exit 1
	fi

	if [[ ${attempt} -gt 0 ]]; then
		echo "[+] $(green 'Completed polling machines')"
		juju machines -m controller 2>&1 | sed 's/^/    | /g'

		sleep "${SHORT_TIMEOUT}"
	fi
>>>>>>> 3.6
}

wait_for_controller_leader() {
	# Since the institution of Dqlite for leases, we need to wait until the
	# backstop workflow has run before we are functional with a single
	# controller.
	# A proxy for this is leadership determination. The command below will
	# sometimes block for extended periods, other times we will be told that
	# leadership can not be determined, so there is no fixed number of attempts
	# that we can rely on.
	# shellcheck disable=SC2143
	until [[ "$(juju exec -m controller --unit controller/leader uptime | grep load)" ]]; do
		echo "[+] waiting for controller leadership"
	done
}

run_controller_limit_access_in_ha() {
	case "${BOOTSTRAP_PROVIDER:-}" in
	"ec2" | "gce")
		machine_info="$(juju list-machines -m controller --format=json)"
		instance_id="$(yq -r '.machines["0"]."instance-id"' <<<"$machine_info")"
		region_or_az=$(region_or_availability_zone)
		network_tag_or_group=$(instance_network_tag_or_group)

		echo "Limit access to all controllers in HA"
		juju expose -m controller controller --to-cidrs 10.0.0.0/24
		wait_for_or_fail "! timeout 5 juju status"

		echo "Temporarily grant this machine access to the 1st controller in the HA"
		allow_access_to_api_port "${instance_id}" "${region_or_az}" "${network_tag_or_group}"
		wait_for_or_fail "timeout 5 juju status"

		echo "Allow access to all controller in HA from anywhere"
		juju expose -m controller controller --to-cidrs 0.0.0.0/0

		# Juju should be able to dump status after removing the temporary network tag
		# to avoid affecting subsequent tests.
		remove_access_to_api_port "${instance_id}" "${region_or_az}" "${network_tag_or_group}"
		wait_for_or_fail "timeout 5 juju status"
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

	juju deploy ubuntu-lite

	enable_microceph_backed_storage

	juju add-unit -m controller controller -n 2

	wait_for_controller_machines 3
	wait_for_ha 3

	# Ensure all the units are fully deployed before trying to
	# tear down HA. There is a window between when wait_for_ha
	# returns and the controller units are fully deployed when
	# remove-machine will fail. Wait for the config to be
	# settled before trying to tear down.
	juju switch controller
	wait_for "controller" "$(idle_condition "controller" 0)"
	wait_for "controller" "$(idle_condition "controller" 1)"
	wait_for "controller" "$(idle_condition "controller" 2)"

	# TODO - split out to separate test.
	# Run limit api port access
	# run_controller_limit_access_in_ha

	juju switch enable-ha
	controller_1=$(juju status -m controller --format json | jq -r '.applications.controller.units["controller/1"].machine')
	juju remove-machine -m controller "${controller_1}" --force
	controller_2=$(juju status -m controller --format json | jq -r '.applications.controller.units["controller/2"].machine')
	juju remove-machine -m controller "${controller_2}" --force

	wait_for_controller_no_leader
	wait_for_controller_leader

	# Ensure that we have no ha enabled machines.
	juju show-controller --format=json | yq -r '.[] | .["controller-machines"] | (.[] | select(.["instance-id"] == null)) as $i ireduce (0; . + 1)' | grep 0

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
