run_prometheus() {
	echo

	MODEL_NAME="test-prometheus"
	file="${TEST_DIR}/${MODEL_NAME}.log"
	bootstrap "${MODEL_NAME}" "${file}"

	juju offer controller.controller:metrics-endpoint

	juju deploy prometheus-k8s --trust
	juju relate prometheus-k8s controller.controller
	wait_for "prometheus-k8s" "$(active_condition "prometheus-k8s")"
	retry check_prometheus_targets 10

	# TODO: need to destroy persistent storage volume here
  destroy_model "${MODEL_NAME}"
	destroy_controller "${MODEL_NAME}"
}

# Check the controller charm can handle multiple Prometheus relations.
run_prometheus_multi() {
	echo

	MODEL_NAME="test-prometheus-multi"
	file="${TEST_DIR}/${MODEL_NAME}.log"
	bootstrap "${MODEL_NAME}" "${file}"

	juju offer controller.controller:metrics-endpoint

	juju deploy prometheus-k8s p1 --trust
	juju relate prometheus-k8s controller.controller
	wait_for "p1" "$(active_condition "p1")"
  retry check_prometheus_targets 10

	juju deploy prometheus-k8s p2 --trust
	juju relate prometheus-k8s controller.controller

	wait_for "p1" "$(active_condition "p1")"
	retry check_prometheus_targets 10

	juju add-unit p1
	retry check_prometheus_targets 10

	# TODO: need to destroy persistent storage volume here
  destroy_model "${MODEL_NAME}"
	destroy_controller "${MODEL_NAME}"
}

# Check the Juju controller is in the list of Prometheus targets.
check_prometheus_targets() {
	set -o pipefail

	PROM_IP=$(juju status --format json | jq -r '.applications."prometheus-k8s".address')
	TARGET=$(curl -sSm 2 "http://${PROM_IP}:9090/api/v1/targets" |
		jq '.data.activeTargets[] | select(.labels.juju_application == "controller")')

	if [[ -z $TARGET ]]; then
		echo "Juju controller not found in Prometheus targets"
		return 1
	fi

	TARGET_STATUS=$(echo $TARGET | jq -r '.health')
	if [[ $TARGET_STATUS != "up" ]]; then
		echo "Controller metrics endpoint status: $TARGET_STATUS: $(echo $TARGET | jq -r '.lastError')"
		return 1
	fi

	echo "Controller metrics endpoint is up"
}

test_prometheus() {
	if [ "$(skip 'test_prometheus')" ]; then
		echo "==> TEST SKIPPED: Prometheus integration"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		case "${BOOTSTRAP_PROVIDER:-}" in
		"k8s")
			run "run_prometheus"
			#run "run_prometheus_multi"
			;;
		*)
			echo "==> TEST SKIPPED: run_prometheus test runs on k8s only"
			;;
		esac

		# TODO: test relating to multiple Prometheus instances at once
		# TODO: test cross-controller relation (lxd controller)
	)
}
