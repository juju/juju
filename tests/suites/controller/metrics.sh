run_metrics() {
	echo

	juju switch controller
	# The controller runs as the jujud snap in a separate process from the
	# machine agent. The jujud.metrics snap app targets the controller's
	# introspection socket; the juju_metrics profile helper targets the
	# machine agent, which does not run the API server.
	OUT=$(
		juju ssh 0 <<EOF
sudo snap run jujud.metrics
EOF
	)
	check_contains "${OUT}" juju_apiserver_connections

}

test_metrics() {
	if [ -n "$(skip 'test_metrics')" ]; then
		echo "==> SKIP: Asked to skip controller metrics tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_metrics"
	)
}
