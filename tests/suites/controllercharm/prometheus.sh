run_prometheus() {
	echo

	file="${TEST_DIR}/test-prometheus.log"
	bootstrap "test-prometheus" "${file}"

	juju offer controller.controller:metrics-endpoint
	juju deploy prometheus-k8s --trust
	juju relate prometheus-k8s controller.controller
	wait_for "prometheus-k8s" "$(idle_condition "prometheus-k8s")"

	retry check_prometheus_targets 10

	destroy_controller "test-prometheus"
}

# Check the Juju controller is in the list of Prometheus targets.
check_prometheus_targets() {
	PROM_IP=$(juju status --format json | jq -r '.applications."prometheus-k8s".address')
	TARGET=$(curl -s "http://${PROM_IP}:9090/api/v1/targets" |
		jq '.data.activeTargets[] | select(.labels.juju_application == "controller")')

	if [[ -z $TARGET ]]; then
		echo "Juju controller not found in Prometheus targets"
		return 1
	fi

	TARGET_STATUS=$(echo $TARGET | jq '.health')
	if [[ $TARGET_STATUS != "up" ]]; then
		echo "Controller metrics endpoint status: $TARGET_STATUS: $(echo $TARGET | jq '.lastError')"
		return 1
	fi
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
			;;
		*)
			echo "==> TEST SKIPPED: run_prometheus test runs on k8s only"
			;;
		esac

		# TODO: test relating to multiple Prometheus instances at once
		# TODO: test cross-controller relation (lxd controller)
	)
}
