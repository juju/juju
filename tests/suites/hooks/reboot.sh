run_start_hook_fires_after_reboot() {
	echo

	model_name="test-start-hook-fires-after-reboot"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# the log messages the test looks for do not appear if root
	# log level is WARNING.
	juju model-config -m "${model_name}" logging-config="<root>=INFO;unit=DEBUG"

	charm="juju-qa-test"
	juju deploy "$charm" --revision 22 --channel stable "$charm"
	wait_for "$charm" "$(idle_condition "$charm")"

	# Ensure that the implicit start hook after reboot detection does not
	# fire for the initial charm deployment
	echo "[+] ensuring that implicit start hook does not fire after initial deployment"
	logs=$(juju debug-log --include-module juju.worker.uniter --replay --no-tail | grep -n "reboot detected" || true)
	echo "$logs" | sed 's/^/    | /g'
	if [ -n "$logs" ]; then
		# shellcheck disable=SC2046
		echo $(red "Uniter incorrectly assumed a reboot occurred after initial charm deployment")
		exit 1
	fi

	# Restart the agent and ensure that the implicit start hook still
	# does not fire. In juju 2.9+, we use a unified agent so we need to restart
	# the machine agent.
	echo "[+] ensuring that implicit start hook does not fire after restarting the (unified) unit agent"
	juju ssh juju-qa-test/0 'sudo service jujud-machine-0 restart'
	echo
	wait_for "$charm" "$(charm_rev "$charm" 22)"
	logs=$(juju debug-log --include-module juju.worker.uniter --replay --no-tail | grep -n "reboot detected" || true)
	echo "$logs" | sed 's/^/    | /g'
	if [ -n "$logs" ]; then
		# shellcheck disable=SC2046
		echo $(red "Uniter incorrectly assumed a reboot occurred after restarting the agent")
		exit 1
	fi
	sleep 1
	wait_for "$charm" "$(idle_condition "$charm")"

	# Ensure that the implicit start hook does not fire after upgrading the unit
	juju refresh juju-qa-test --revision 23
	echo
	sleep 1
	wait_for "$charm" "$(charm_rev "$charm" 23)"
	logs=$(juju debug-log --include-module juju.worker.uniter --replay --no-tail | grep -n "reboot detected" || true)
	echo "$logs" | sed 's/^/    | /g'
	if [ -n "$logs" ]; then
		# shellcheck disable=SC2046
		echo $(red "Uniter incorrectly assumed a reboot occurred after restarting the agent")
		exit 1
	fi

	sleep 1
	wait_for "$charm" "$(idle_condition "$charm")"

	# Trigger a reboot and verify that the implicit start hook fires
	echo "[+] ensuring that implicit start hook fires after a machine reboot"
	juju ssh juju-qa-test/0 'sudo reboot now' || true
	sleep 1
	wait_for "$charm" "$(idle_condition "$charm")"
	echo
	logs=$(juju debug-log --include-module juju.worker.uniter --replay --no-tail | grep -n "reboot detected" || true)
	echo "$logs" | sed 's/^/    | /g'
	if [ -z "$logs" ]; then
		# shellcheck disable=SC2046
		echo $(red "Uniter did not fire start hook after the machine rebooted")
		exit 1
	fi

	destroy_model "${model_name}"
}

run_reboot_monitor_state_cleanup() {
	echo

	model_name="test-reboot-monitor-state-cleanup"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju deploy juju-qa-test --base ubuntu@22.04
	juju deploy juju-qa-dummy-subordinate
	juju integrate juju-qa-test dummy-subordinate
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"
	wait_for "dummy-subordinate" "$(idle_subordinate_condition "dummy-subordinate" "juju-qa-test")"

	# Check that the reboot flag files have been created for both the charm and
	# the subordinate. Note: juju ssh adds whitespace which we need to trim
	# with a bit of awk magic to ensure that our comparisons work correctly
	echo "[+] Verifying that reboot monitor state files are in place"
	num_files=$(juju ssh juju-qa-test/0 'ls -1 /var/run/juju/reboot-monitor/ | wc -l' 2>/dev/null | tr -d "[:space:]")
	echo "   | number of monitor state files: ${num_files}"
	if [ "$num_files" != "2" ]; then
		# shellcheck disable=SC2046
		echo $(red "Expected 2 reboot monitor state files to be created; got ${num_files}")
		exit 1
	fi

	# Remove subordinate and ensure that the state file for its monitor got purged
	echo "[+] Verifying that reboot monitor state files are removed once a subordinate gets removed"
	juju remove-relation juju-qa-test dummy-subordinate
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"

	wait_for_subordinate_count "juju-qa-test"
	num_files=$(juju ssh juju-qa-test/0 'ls -1 /var/run/juju/reboot-monitor/ | wc -l' 2>/dev/null | tr -d "[:space:]")
	echo "   | number of monitor state files: ${num_files}"
	if [ "$num_files" != "1" ]; then
		# shellcheck disable=SC2046
		echo $(red "Expected one remaining reboot monitor state file after subordinate removal; got ${num_files}")
		exit 1
	fi

	destroy_model "${model_name}"
}

test_start_hook_fires_after_reboot() {
	if [ "$(skip 'test_start_hook_fires_after_reboot')" ]; then
		echo "==> TEST SKIPPED: start hook fires after reboot"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_start_hook_fires_after_reboot"
		run "run_reboot_monitor_state_cleanup"
	)
}
