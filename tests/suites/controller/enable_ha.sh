wait_for_controller_machines_tear_down() {
	amount=${1}

	attempt=0
	# shellcheck disable=SC2143
	until [[ "$(juju machines -m controller --format=json | jq -r '.machines | .[] | .["juju-status"] | select(.current == "started") | .current' | wc -l | grep "${amount}")" ]]; do
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
	until [[ "$(juju machines -m controller --format=json | jq -r '.machines | .[] | .["juju-status"] | select(.current == "stopped") | .current' | wc -l | grep 0)" ]]; do
		echo "[+] (attempt ${attempt}) polling stopped machines during ha tear down"
		juju machines -m controller 2>&1 | sed 's/^/    | /g' || true
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))

		if [[ ${attempt} -gt 25 ]]; then
			echo "enable-ha failed waiting for machines to tear down"
			exit 1
		fi
	done

	if [[ "$(juju machines -m controller --format=json | jq -r '.machines | .[] | .["juju-status"] | select(.current == "error") | .current' | wc -l)" -gt 0 ]]; then
		echo "machine in controller model with error during ha tear down"
		juju machines -m controller 2>&1 | sed 's/^/    | /g' || true
		exit 1
	fi

	if [[ ${attempt} -gt 0 ]]; then
		echo "[+] $(green 'Completed polling machines')"
		juju machines -m controller 2>&1 | sed 's/^/    | /g'

		sleep "${SHORT_TIMEOUT}"
	fi
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

	juju deploy jameinel-ubuntu-lite

	juju enable-ha

	wait_for_controller_machines 3
	wait_for_ha 3

	# Ensure all the units are fully deployed before trying to
	# tear down HA. There is a window between when wait_for_ha
	# returns and the controller units are fully deployed when
	# remove-machine will fail. Wait for the config to be
	# settled before trying to tear down.
	juju switch controller
	wait_for "controller" "$(idle_condition "controller" 0 0)"
	wait_for "controller" "$(idle_condition "controller" 0 1)"
	wait_for "controller" "$(idle_condition "controller" 0 2)"

	juju switch enable-ha
	juju remove-machine -m controller 1
	juju remove-machine -m controller 2

	wait_for_controller_machines_tear_down 1

	# Ensure that we have no ha enabled machines.
	juju show-controller --format=json | jq -r '.[] | .["controller-machines"] |  reduce(.[] | select(.["instance-id"] == null)) as $i (0;.+=1)' | grep 0

	wait_for_controller_leader

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
