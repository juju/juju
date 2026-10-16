run_secrets_cmr() {
	echo

	echo "First set up a cross model relation"
	add_model "model-secrets-offer"
	juju --show-log deploy prometheus-k8s source --trust
	juju --show-log offer source:self-metrics-endpoint
	wait_for "source" "$(idle_condition "source")"

	add_model "model-secrets-consume"
	juju --show-log deploy prometheus-k8s sink --trust
	juju --show-log integrate sink:metrics-endpoint model-secrets-offer.source
	wait_for "sink" "$(idle_condition "sink")"

	juju switch "model-secrets-offer"
	wait_for "1" '.offers["source"]["active-connected-count"]'

	echo "Create and share a secret on the offer side"
	secret_uri=$(juju_exec_output --unit source/0 -- secret-add foo=bar)
	relation_id=$(juju --show-log show-unit -m model-secrets-offer source/0 --format json | yq '."source/0"."relation-info" | .[] | select(."endpoint"=="self-metrics-endpoint") | ."relation-id"')
	juju exec --unit source/0 -- secret-grant "$secret_uri" -r "$relation_id"

	echo "Checking: the secret can be read by the consumer"
	juju switch "model-secrets-consume"
	echo "Checking:  secret-get by URI - consume content"
	check_contains "$(juju_exec_output --unit sink/0 -- secret-get --label mylabel "$secret_uri")" 'foo: bar'
	echo "Checking:  secret-get by URI - consume content"
	check_contains "$(juju_exec_output --unit sink/0 -- secret-get --label mylabel)" 'foo: bar'

	echo "Checking: add a new revision and check consumer can see it"
	juju switch "model-secrets-offer"
	juju exec --unit source/0 -- secret-set "$secret_uri" foo=bar2
	juju switch "model-secrets-consume"
	check_contains "$(juju_exec_output --unit sink/0 -- secret-get --label mylabel)" 'foo: bar'
	check_contains "$(juju_exec_output --unit sink/0 -- secret-get --label mylabel --peek)" 'foo: bar2'
	check_contains "$(juju_exec_output --unit sink/0 -- secret-get --label mylabel)" 'foo: bar'
	check_contains "$(juju_exec_output --unit sink/0 -- secret-get --label mylabel --refresh)" 'foo: bar2'
	check_contains "$(juju_exec_output --unit sink/0 -- secret-get --label mylabel)" 'foo: bar2'

	echo "Checking: suspend relation and check access is lost"
	juju switch "model-secrets-offer"
	juju suspend-relation "$relation_id"
	juju switch "model-secrets-consume"
	check_contains "$(juju_exec_output --unit sink/0 -- secret-get "$secret_uri" 2>&1)" 'permission denied'
	echo "Checking: resume relation and access is restored"
	juju switch "model-secrets-offer"
	juju resume-relation "$relation_id"
	juju switch "model-secrets-consume"
	check_contains "$(juju_exec_output --unit sink/0 -- secret-get --label mylabel)" 'foo: bar2'

	echo "Checking: secret-revoke by relation ID"
	juju switch "model-secrets-offer"
	juju exec --unit source/0 -- secret-revoke "$secret_uri" --relation "$relation_id"
	juju switch "model-secrets-consume"
	check_contains "$(juju_exec_output --unit sink/0 -- secret-get "$secret_uri" 2>&1)" 'permission denied'
}

test_secrets_cmr() {
	if [ "$(skip 'test_secrets_cmr')" ]; then
		echo "==> TEST SKIPPED: test_secrets_cmr"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_secrets_cmr"
	)
}

run_secrets_cmr_cross_controller() {
	echo

	# The offering model's built-in k8s secret backend records the endpoint
	# of the built-in microk8s cloud, taken from the host-side kubeconfig
	# (https://127.0.0.1:16443) and only reachable from the host. A consumer
	# on another controller must instead be handed the in-cluster address,
	# otherwise its secret reads fail forever with "connection refused".
	# This requires a second controller on the same microk8s cluster, so
	# that the in-cluster address is reachable from the consumer's pod.

	echo "Set up the offering model and the offer"
	add_model "model-secrets-offer-xc"
	juju --show-log deploy prometheus-k8s source --trust
	juju --show-log offer source:self-metrics-endpoint
	wait_for "source" "$(idle_condition "source")"

	offer_controller="$(juju controllers --format=yaml | yq -r '."current-controller"')"

	echo "Bootstrap a second controller on the same microk8s cluster"
	bootstrap_alt_controller "secrets-cmr-consumer"

	echo "Deploy the consuming workload"
	juju switch "secrets-cmr-consumer"
	add_model "model-secrets-consume-xc"
	juju --show-log deploy prometheus-k8s sink --trust
	wait_for "sink" "$(idle_condition "sink")"

	echo "Consume the offer across controllers and integrate"
	juju consume "${offer_controller}:admin/model-secrets-offer-xc.source"
	juju --show-log integrate sink:metrics-endpoint source
	# wait for the cross-model relation to be joined. On the sink
	# application the relation map is keyed by its own endpoint
	# ("metrics-endpoint"), with the remote application as the value.
	wait_for "source" '.applications["sink"] | .relations["metrics-endpoint"][0]'

	echo "Create and share a secret on the offer side"
	juju switch "${offer_controller}:model-secrets-offer-xc"
	# Cross-model relations are not listed in the offering application's
	# status relations map: on the offering side they surface through the
	# offers section instead.
	wait_for "1" '.offers["source"]["active-connected-count"]'
	secret_uri=$(juju_exec_output --unit source/0 -- secret-add foo=bar)
	relation_id=$(juju --show-log show-unit -m model-secrets-offer-xc source/0 --format json | yq '."source/0"."relation-info" | .[] | select(."endpoint"=="self-metrics-endpoint") | ."relation-id"')
	if [[ -z ${relation_id} ]]; then
		echo "ERROR: could not extract the offer-side relation id for the"
		echo "    cross-model relation from show-unit source/0"
		exit 1
	fi
	juju exec --unit source/0 -- secret-grant "$secret_uri" -r "$relation_id"

	echo "Checking: the cross-controller consumer can read the secret"
	juju switch "secrets-cmr-consumer:model-secrets-consume-xc"
	check_contains "$(juju_exec_output --unit sink/0 -- secret-get --label mylabel "$secret_uri")" 'foo: bar'
	check_contains "$(juju_exec_output --unit sink/0 -- secret-get --label mylabel)" 'foo: bar'

	echo "Checking: the consumer never used the recorded loopback endpoint"
	check_not_contains "$(juju debug-log --no-tail)" '127.0.0.1:16443'

	echo "Remove the consumed relation and the offer"
	juju remove-relation sink source
	juju remove-saas source
	juju switch "${offer_controller}:model-secrets-offer-xc"
	wait_for null '.offers."source"."total-connected-count"'
	juju remove-offer "${offer_controller}:admin/model-secrets-offer-xc.source" -y

	echo "Clean up the consumer controller"
	destroy_controller "secrets-cmr-consumer"
}

test_secrets_cmr_cross_controller() {
	if [ "$(skip 'test_secrets_cmr_cross_controller')" ]; then
		echo "==> TEST SKIPPED: test_secrets_cmr_cross_controller"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_secrets_cmr_cross_controller"
	)
}
