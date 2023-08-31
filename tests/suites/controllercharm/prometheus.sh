run_prometheus() {
	echo

	file="${TEST_DIR}/test-prometheus.log"
	bootstrap "test-prometheus" "${file}"

	juju switch controller
	juju offer controller:metrics-endpoint
	juju add-model prom
	juju deploy prometheus-k8s p --trust
	juju relate p controller.controller

	# TODO: look at prometheus targets

	destroy_controller "test-prometheus"
}

# Check we can relate the controller charm to multiple Prometheus instances.
run_prometheus_multi() {
	# TODO: fill this in
	:
}

test_prometheus() {
	if [ "$(skip 'test_prometheus')" ]; then
		echo "==> TEST SKIPPED: Prometheus integration"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_prometheus"
		run "run_prometheus_multi"
	)
}
