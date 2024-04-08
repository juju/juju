run_model_multi() {
	# Echo out to ensure nice output to the test suite.
	echo

	# The following ensures that a bootstrap juju exists.
	file="${TEST_DIR}/test-model-multi.log"
	ensure "model-multi" "${file}"

	deploy_stack "test-models"

	juju add-model env1
	deploy_stack "env1"

	juju add-model env2
	deploy_stack "env2"

	check_services "test-models"
	check_services "env1"
	check_services "env2"

	destroy_model "env1"
	destroy_model "env2"
	destroy_model "model-multi"
}

deploy_stack() {
	name=${1}

	juju switch "${name}"

	juju deploy juju-qa-dummy-source --base ubuntu@22.04
	juju deploy juju-qa-dummy-sink --base ubuntu@22.04

	juju integrate dummy-source dummy-sink
	juju expose dummy-sink

	wait_for_machine_agent_status 0 "started"
	wait_for_machine_agent_status 1 "started"
}

check_services() {
	name=${1}

	juju switch "${name}"

	token=$(rnd_str)
	echo "[+] Setting token for ${name} with ${token}"
	juju config dummy-source token="${token}"

	attempt=0
	until [[ $(juju status --format json | jq -er ".applications | .[\"dummy-source\"] | .units | .[\"dummy-source/0\"] | .[\"workload-status\"] | select(.[\"message\"] == \"Token is ${token}\") | .message") ]]; do
		echo "[+] (attempt ${attempt}) polling status"
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))

		if [ "${attempt}" -gt 2 ]; then
			echo "[-] $(red 'Failed polling status')"
			exit 1
		fi
	done

	if [ "${attempt}" -eq 0 ]; then
		echo "[-] $(red 'Failed polling status')"
		exit 1
	fi

	echo "[+] $(green 'Completed polling status')"
}

test_model_multi() {
	if [ -n "$(skip 'test_model_multi')" ]; then
		echo "==> SKIP: Asked to skip model multi tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_model_multi"
	)
}
