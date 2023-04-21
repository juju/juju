cat_query_tracing_enabled_agent_conf() {
	# shellcheck disable=SC2046
	echo $(juju exec -m controller --machine 0 'sudo cat /var/lib/juju/agents/machine-0/agent.conf' | grep "querytracingenabled")
}

run_query_tracing_enabled() {
	echo

	file="${TEST_DIR}/query_tracing_enabled.log"

	ensure "query-tracing-enabled" "${file}"

	check_not_contains "$(cat_query_tracing_enabled_agent_conf)" querytracingenabled

	juju controller-config query-tracing-enabled=true

	sleep 5

	attempt=0
	# shellcheck disable=SC2046,SC2143,SC2091
	until $(check_contains "$(cat_query_tracing_enabled_agent_conf)" "true" >/dev/null 2>&1); do
		echo "[+] (attempt ${attempt}) polling agent conf"
		cat_query_tracing_enabled_agent_conf | sed 's/^/    | /g'
		# This will attempt to wait for 2 minutes before failing out.
		if [[ ${attempt} -ge 24 ]]; then
			echo "Failed: expected querytracingenabled to be set in agent conf."
			exit 1
		fi
		sleep 5
		attempt=$((attempt + 1))
	done

	# Set the value back in case we are reusing a controller
	juju controller-config query-tracing-enabled=false

	sleep 5

	destroy_model "query-tracing-enabled"
}

cat_query_tracing_threshold_agent_conf() {
	# shellcheck disable=SC2046
	echo $(juju exec -m controller --machine 0 'sudo cat /var/lib/juju/agents/machine-0/agent.conf' | grep "querytracingthreshold")
}

run_query_tracing_threshold() {
	echo

	file="${TEST_DIR}/query_tracing_threshold.log"

	ensure "query-tracing-threshold" "${file}"

	check_contains "$(cat_query_tracing_threshold_agent_conf)" "1s"

	juju controller-config query-tracing-threshold="42ms"

	sleep 5

	attempt=0
	# shellcheck disable=SC2046,SC2143,SC2091
	until $(check_contains "$(cat_query_tracing_threshold_agent_conf)" "42ms" >/dev/null 2>&1); do
		echo "[+] (attempt ${attempt}) polling agent conf"
		cat_query_tracing_threshold_agent_conf | sed 's/^/    | /g'
		# This will attempt to wait for 2 minutes before failing out.
		if [[ ${attempt} -ge 24 ]]; then
			echo "Failed: expected querytracingthreshold to be set in agent conf."
			exit 1
		fi
		sleep 5
		attempt=$((attempt + 1))
	done

	# Set the value back in case we are reusing a controller
	juju controller-config query-tracing-threshold="1s"

	sleep 5

	destroy_model "query-tracing-threshold"
}

test_query_tracing() {
	if [[ -n "$(skip 'test_query_tracing')" ]]; then
		echo "==> SKIP: Asked to skip controller query tracing tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_query_tracing_enabled"
		run "run_query_tracing_threshold"
	)
}
