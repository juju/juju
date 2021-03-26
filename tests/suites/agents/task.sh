test_agents() {
	if [ "$(skip 'test_agents')" ]; then
		echo "==> TEST SKIPPED: agent tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-agents.log"

	JUJU_AGENT_TESTING_OPTIONS=CHARM_REVISION_UPDATE_INTERVAL=15s \
		bootstrap "test-agents" "${file}"

	test_charmrevisionupdater

	destroy_controller "test-agents"
}
