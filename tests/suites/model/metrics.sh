run_model_metrics() {
	# Echo out to ensure nice output to the test suite.
	echo

	testname="model-metrics"

	# The following ensures that a bootstrap juju exists.
	file="${TEST_DIR}/test-${testname}.log"
	ensure "${testname}" "${file}"

	# deploy ubuntu with a different name, check that the metric send the charm name, not the application name.
	juju deploy ubuntu app-one --base ubuntu@22.04
	juju deploy juju-qa-test
	juju deploy ntp # ntp currently only works on 22.04 and before.
	juju relate ntp app-one

	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test" 1)"
	wait_for "app-one" "$(idle_condition "app-one" 0)"
	wait_for "ntp" "$(idle_subordinate_condition "ntp" "app-one" 0)"

	juju relate ntp:juju-info juju-qa-test:juju-info
	wait_for "ntp" "$(idle_subordinate_condition "ntp" "juju-qa-test" 1)"

	juju model-config -m controller logging-config="<root>=INFO;#charmhub=TRACE"

	# Restarting the controller service causes the charmrevisionupdater worker to run.
	juju ssh -m controller 0 -- sudo systemctl restart jujud-machine-0.service

	echo "Sleep 45, give charmrevisionupdater time to kick off after controller jujud restart."
	sleep 45

	attempt=0
	while true; do
		OUT=$(juju debug-log -m controller --include-module juju.apiserver.charmrevisionupdater.client | grep metrics || true)
		if echo "${OUT}" | grep -e '"metrics":{"relations":"juju-qa-test,ubuntu","units":"2"}' -e '"metrics":{"relations":"ntp","units":"1"}' -e '"model":{"applications":"3",' -e '"machines":"2",'; then
			break
		fi
		echo "${OUT}"
		attempt=$((attempt + 1))
		if [ $attempt -eq 10 ]; then
			# shellcheck disable=SC2046
			echo $(red "timeout: waiting for metrics in debug log 50sec")
			exit 5
		fi
		sleep 5
	done

	# clean up
	juju model-config -m controller logging-config="<root>=INFO"
	destroy_model "${testname}"
}

run_empty_model_metrics() {
	# Echo out to ensure nice output to the test suite.
	echo

	testname="empty-model-metrics"

	# The following ensures that a bootstrap juju exists.
	file="${TEST_DIR}/test-${testname}.log"
	ensure "${testname}" "${file}"

	juju add-machine -n 3
	wait_for_machine_agent_status 0 "started"
	wait_for_machine_agent_status 1 "started"
	wait_for_machine_agent_status 2 "started"
	juju model-config -m controller logging-config="<root>=INFO;#charmhub=TRACE"

	# Restarting the controller service causes the charmrevisionupdater worker to run.
	juju ssh -m controller 0 -- sudo systemctl restart jujud-machine-0.service

	echo "Sleep 45, give charmrevisionupdater time to kick off after controller jujud restart."
	sleep 45

	attempt=0
	while true; do
		OUT=$(juju debug-log -m controller --include-module juju.apiserver.charmrevisionupdater.client --replay | grep metrics || true)
		if echo "${OUT}" | grep '"machines":"3"'; then
			break
		fi
		echo "${OUT}"
		attempt=$((attempt + 1))
		if [ $attempt -eq 10 ]; then
			# shellcheck disable=SC2046
			echo $(red "timeout: waiting for empty model metrics in debug log 50sec")
			exit 5
		fi
		sleep 5
	done

	# clean up
	juju model-config -m controller logging-config="<root>=INFO"
	destroy_model "${testname}"
}

run_model_metrics_disabled() {
	# Echo out to ensure nice output to the test suite.
	echo

	testname="model-metrics-disabled"

	# The following ensures that a bootstrap juju exists.
	file="${TEST_DIR}/test-${testname}.log"
	ensure "${testname}" "${file}"

	juju deploy ubuntu -n 2
	wait_for "ubuntu" "$(idle_condition "ubuntu" 0 0)"
	wait_for "ubuntu" "$(idle_condition "ubuntu" 0 1)"
	juju model-config disable-telemetry=true
	juju model-config -m controller logging-config="<root>=INFO;#charmhub=TRACE"

	# Restarting the controller service causes the charmrevisionupdater worker to run.
	juju ssh -m controller 0 -- sudo systemctl restart jujud-machine-0.service

	echo "Sleep 120, give charmrevisionupdater time to kick off after controller jujud restart."
	sleep 120

	attempt=0
	while true; do
		OUT=$(juju debug-log -m controller --include-module juju.apiserver.charmrevisionupdater.client --replay | grep metrics || true)
		if echo "${OUT}" | grep "metrics" | grep -v '"machines":"2"'; then
			break
		fi
		echo "${OUT}"
		attempt=$((attempt + 1))
		if [ $attempt -eq 10 ]; then
			# shellcheck disable=SC2046
			echo $(red "timeout: waiting for no model metrics in debug log 50sec")
			exit 5
		fi
		sleep 5
	done

	# clean up
	juju model-config -m controller logging-config="<root>=INFO"
	destroy_model "${testname}"
}

test_model_metrics() {
	if [ -n "$(skip 'test_model_metrics')" ]; then
		echo "==> SKIP: Asked to skip model metrics tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_model_metrics_disabled"
		run "run_empty_model_metrics"
		run "run_model_metrics"
	)

}
