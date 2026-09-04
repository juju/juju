wait_for_ha_teardown() {
	attempt=0
	# After tearing down HA, wait for the surviving controller machine to
	# be the only one left and to keep its dqlite voter role. Status for
	# the controller model can only be served once quorum is restored, so
	# reaching this state also implies that the dqlite backstop has
	# reconfigured the cluster around the surviving node.
	# shellcheck disable=SC2143
	until status=$(timeout 10 juju status -m controller --format=json 2>/dev/null) &&
		count=$(yq -r '.machines | to_entries | length' <<<"${status}") &&
		voters=$(yq -r '.machines | to_entries[] | select(.value["controller-cluster-role"] == "voter") | .key' <<<"${status}" | wc -l) &&
		[[ ${count} -eq 1 && ${voters} -eq 1 ]]; do
		echo "[+] (attempt ${attempt}) polling ha teardown"
		juju status -m controller --format=yaml 2>&1 | yq '.machines | with_entries(.value |= pick(["instance-id", "controller-cluster-role"]))' 2>&1 | sed 's/^/    | /g' || true
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))

		if [[ ${attempt} -gt 100 ]]; then
			echo "high availability teardown failed waiting for a single controller"
			exit 1
		fi
	done

	if [[ ${attempt} -gt 0 ]]; then
		echo "[+] $(green 'Completed polling ha teardown')"
		juju status -m controller --format=yaml 2>&1 | yq '.machines | with_entries(.value |= pick(["instance-id", "controller-cluster-role"]))' 2>&1 | sed 's/^/    | /g'

		sleep "${SHORT_TIMEOUT}"
	fi
}

wait_for_controller_leadership() {
	attempt=0
	# Leadership determination requires quorum and a functional lease
	# manager, so it only succeeds once the dqlite backstop has
	# reconfigured the cluster around the surviving controller.
	# The timeout bounds each attempt so a broken backstop fails the
	# test instead of hanging it.
	# shellcheck disable=SC2143
	# Not using juju_exec_output: uptime is a plain system command that
	# produces no stderr, so the stdout/stderr mixing bug does not apply.
	until timeout 60 juju exec -m controller --unit controller/leader uptime 2>/dev/null | grep load; do
		echo "[+] (attempt ${attempt}) waiting for controller leadership"
		attempt=$((attempt + 1))

		if [[ ${attempt} -gt 12 ]]; then
			echo "controller leadership not restored after HA teardown"
			exit 1
		fi
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

	juju deploy ubuntu-lite

	# TODO - a separate ha test with object storage
	# enable_microceph_backed_storage

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
	controller_1=$(juju status -m controller --format json | yq -r '.applications.controller.units["controller/1"].machine')
	controller_2=$(juju status -m controller --format json | yq -r '.applications.controller.units["controller/2"].machine')
	juju remove-machine -m controller "${controller_1}" --force --no-prompt
	wait_for_ha 2
	juju remove-machine -m controller "${controller_2}" --force --no-prompt
	wait_for_ha 1

	wait_for_ha_teardown

	# The machine view above converges as soon as remove-machine records
	# the deletions, well before the controller unit and its charm hooks have
	# settled. Wait for the surviving unit to be idle before probing dqlite
	# leadership, otherwise the probe races the dqlite backstop recovery.
	juju switch controller
	wait_for "controller" "$(idle_condition "controller" 0)" 900
	wait_for_controller_leadership

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
