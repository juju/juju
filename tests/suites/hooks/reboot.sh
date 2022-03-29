run_start_hook_fires_after_reboot() {
	echo

	model_name="test-start-hook-fires-after-reboot"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# the log messages the test looks for do not appear if root
	# log level is WARNING.
	juju model-config -m "${model_name}" logging-config="<root>=INFO;unit=DEBUG"

	juju deploy cs:~jameinel/ubuntu-lite-6
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"

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
	juju ssh ubuntu-lite/0 'sudo service jujud-machine-0 restart'
	echo
	wait_for "ubuntu-lite" "$(charm_rev "ubuntu-lite" 6)"
	logs=$(juju debug-log --include-module juju.worker.uniter --replay --no-tail | grep -n "reboot detected" || true)
	echo "$logs" | sed 's/^/    | /g'
	if [ -n "$logs" ]; then
		# shellcheck disable=SC2046
		echo $(red "Uniter incorrectly assumed a reboot occurred after restarting the agent")
		exit 1
	fi
	sleep 1
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"

	# Ensure that the implicit start hook does not fire after upgrading the unit
	juju upgrade-charm ubuntu-lite --revision 7
	echo
	sleep 1
	wait_for "ubuntu-lite" "$(charm_rev "ubuntu-lite" 7)"
	logs=$(juju debug-log --include-module juju.worker.uniter --replay --no-tail | grep -n "reboot detected" || true)
	echo "$logs" | sed 's/^/    | /g'
	if [ -n "$logs" ]; then
		# shellcheck disable=SC2046
		echo $(red "Uniter incorrectly assumed a reboot occurred after restarting the agent")
		exit 1
	fi

	sleep 1
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"

	# Trigger a reboot and verify that the implicit start hook fires
	echo "[+] ensuring that implicit start hook fires after a machine reboot"
	juju ssh ubuntu-lite/0 'sudo reboot now' || true
	sleep 1
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"
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

	# Deploy mysql/rsyslog-forwarder-ha. The latter is a subordinate
	juju deploy mysql
	juju deploy rsyslog-forwarder-ha
	juju relate rsyslog-forwarder-ha mysql
	wait_for "mysql" "$(idle_condition "mysql")"
	wait_for "rsyslog-forwarder-ha" "$(idle_subordinate_condition "rsyslog-forwarder-ha" "mysql")"

	# Check that the reboot flag files have been created for both the charm and
	# the subordinate. Note: juju ssh adds whitespace which we need to trim
	# with a bit of awk magic to ensure that our comparisons work correctly
	echo "[+] Verifying that reboot monitor state files are in place"
	num_files=$(juju ssh mysql/0 'ls -1 /var/run/juju/reboot-monitor/ | wc -l' 2>/dev/null | tr -d "[:space:]")
	echo "   | number of monitor state files: ${num_files}"
	if [ "$num_files" != "2" ]; then
		# shellcheck disable=SC2046
		echo $(red "Expected 2 reboot monitor state files to be created; got ${num_files}")
		exit 1
	fi

	# Remove subordinate and ensure that the state file for its monitor got purged
	echo "[+] Verifying that reboot monitor state files are removed once a subordinate gets removed"
	juju remove-relation rsyslog-forwarder-ha mysql
	wait_for "mysql" "$(idle_condition "mysql")"

	wait_for_subordinate_count "mysql"
	num_files=$(juju ssh mysql/0 'ls -1 /var/run/juju/reboot-monitor/ | wc -l' 2>/dev/null | tr -d "[:space:]")
	echo "   | number of monitor state files: ''${num_files}"
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
