run_metrics() {
	echo

	juju switch controller
	OUT=$(
		juju ssh 0 <<EOF
. /etc/profile.d/juju-introspection.sh
juju_metrics
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
