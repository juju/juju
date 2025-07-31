wait_for_controller_no_leader() {
	# We need to wait for the Dqlite cluster to be broken (loss of quorum),
	# before we start waiting for the backstop behaviour to be pending
	# (see wait_for_controller_leader below).
	# shellcheck disable=SC2143
	until ! [[ "$(juju exec -m controller --unit controller/leader uptime | grep load)" ]]; do
		echo "[+] waiting for no controller leadership"
	done
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

	juju switch enable-ha
	controller_1=$(juju status -m controller --format json | jq -r '.applications.controller.units["controller/1"].machine')
	juju remove-machine -m controller "${controller_1}" --force
	controller_2=$(juju status -m controller --format json | jq -r '.applications.controller.units["controller/2"].machine')
	juju remove-machine -m controller "${controller_2}" --force

	wait_for_controller_no_leader
	wait_for_controller_leader

	# Ensure that we have no ha enabled machines.
	juju show-controller --format=json | jq -r '.[] | .["controller-machines"] |  reduce(.[] | select(.["instance-id"] == null)) as $i (0;.+=1)' | grep 0

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
