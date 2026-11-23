run_relation_settings_ordering() (
	local model_name relation_id sink_service
	model_name="test-relation-settings-ordering"
	# ensure adds the client key with juju add-ssh-key before deploying units.
	ensure "${model_name}" "${TEST_DIR}/${model_name}.log"

	juju deploy juju-qa-dummy-source dummy-source --base ubuntu@22.04 --config token=ordering-A
	juju deploy juju-qa-dummy-sink dummy-sink --base ubuntu@22.04
	juju integrate dummy-source:sink dummy-sink:source

	wait_for "ordering-A" "$(workload_status dummy-sink 0).message"
	wait_for "dummy-source" "$(active_idle_condition dummy-source)"
	wait_for "dummy-sink" "$(active_idle_condition dummy-sink)"
	relation_id=$(juju_exec_output --unit dummy-source/0 -- 'relation-ids sink')
	# Allow direct SSH while the agent-backed SSH tunnel is stopped.
	juju ssh-keys --full | sed '1d' | juju ssh dummy-sink/0 'cat >> ~/.ssh/authorized_keys'
	# The sink's unit workers run inside its machine agent.
	sink_service=$(juju ssh --direct --proxy=false dummy-sink/0 \
		"systemctl list-unit-files --type=service --no-legend 'jujuagentd-machine-*.service' | awk '{print \$1}'")
	[[ -n ${sink_service} && ${sink_service} != *$'\n'* ]]

	# Commit A -> B -> A while the sink cannot observe the intermediate value.
	# Capture local names now so the EXIT trap can restore the agent on failure.
	# shellcheck disable=SC2064
	trap "juju ssh --direct --proxy=false -m '${model_name}' dummy-sink/0 'sudo systemctl start ${sink_service}'" EXIT
	juju ssh --direct --proxy=false dummy-sink/0 "sudo systemctl stop ${sink_service}"

	# Removing the hook's output proves a fresh hook runs after the restart;
	# checking status alone would accept the original token A.
	juju ssh --direct --proxy=false dummy-sink/0 "sudo rm /var/run/dummy-sink/token"
	juju_exec_output --unit dummy-source/0 -- "relation-set -r ${relation_id} token=ordering-B"
	juju_exec_output --unit dummy-source/0 -- "relation-set -r ${relation_id} token=ordering-A"

	juju ssh --direct --proxy=false dummy-sink/0 "sudo systemctl start ${sink_service}"
	trap - EXIT
	wait_for_or_fail 'juju ssh --direct --proxy=false dummy-sink/0 "test -f /var/run/dummy-sink/token"' 120
	wait_for "dummy-sink" "$(active_idle_condition dummy-sink)"
	juju_exec_output --unit dummy-sink/0 -- 'cat /var/run/dummy-sink/token' | check "ordering-A"

	destroy_model "${model_name}"
)

test_relation_settings_ordering() {
	if [ "$(skip 'test_relation_settings_ordering')" ]; then
		echo "==> TEST SKIPPED: relation settings ordering"
		return
	fi
	(
		set_verbosity
		cd .. || exit
		run "run_relation_settings_ordering"
	)
}
