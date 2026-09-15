# count_secret_changed_hooks returns the number of times the given unit has
# run the secret-changed hook, as recorded in the unit's status history.
count_secret_changed_hooks() {
	local model=$1 unit=$2
	juju show-status-log -m "${model}" "${unit}" --format json -n 500 |
		yq -r "[.[] | select(.message != null) | select(.message | contains(\"running secret-changed hook\"))] | length" 2>/dev/null || echo 0
}

# wait_for_secret_changed_hooks waits until the given unit has run at least
# expected secret-changed hooks.
wait_for_secret_changed_hooks() {
	local model=$1 unit=$2 expected=$3
	attempt=0
	while true; do
		num_hooks=$(count_secret_changed_hooks "${model}" "${unit}")
		if [ "${num_hooks}" -ge "${expected}" ]; then
			echo "==> ${unit} on ${model} has run ${num_hooks} secret-changed hook(s)"
			return 0
		fi
		attempt=$((attempt + 1))
		if [ ${attempt} -eq 40 ]; then
			# shellcheck disable=SC2046
			echo $(red "expected at least ${expected} secret-changed hooks for ${model}/${unit}, got ${num_hooks}")
			exit 1
		fi
		sleep 5
	done
}

# wait_for_hook_count_stable waits until the secret-changed hook count of a
# unit stops increasing, so that a baseline count is reliable. Registration of
# a consumed secret can fire a secret-changed hook asynchronously.
wait_for_hook_count_stable() {
	local model=$1 unit=$2
	attempt=0
	previous=-1
	while true; do
		current=$(count_secret_changed_hooks "${model}" "${unit}")
		if [ "${current}" -eq "${previous}" ]; then
			echo "==> secret-changed hook count for ${unit} on ${model} is stable at ${current}"
			return 0
		fi
		previous=${current}
		attempt=$((attempt + 1))
		if [ ${attempt} -eq 6 ]; then
			# shellcheck disable=SC2046
			echo $(red "secret-changed hook count for ${model}/${unit} did not stabilise (${current})")
			exit 1
		fi
		sleep 5
	done
}

run_secrets_cmr() {
	echo

	echo "First set up a cross model relation"
	add_model "model-secrets-offer"
	juju --show-log deploy juju-qa-dummy-source
	juju --show-log offer dummy-source:sink
	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	add_model "model-secrets-consume"
	juju --show-log deploy juju-qa-dummy-sink
	juju --show-log integrate dummy-sink model-secrets-offer.dummy-source

	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"
	wait_for "dummy-source" '.applications["dummy-sink"] | .relations.source[0]'

	juju switch "model-secrets-offer"
	juju config dummy-source token=foobar
	juju switch "model-secrets-consume"
	wait_for "active" '."application-endpoints"["dummy-source"]."application-status".current'

	juju switch "model-secrets-offer"
	wait_for "1" '.offers["dummy-source"]["active-connected-count"]'

	echo "Create and share a secret on the offer side"
	secret_uri=$(juju_exec_output --unit dummy-source/0 -- secret-add foo=bar)
	relation_id=$(juju --show-log show-unit -m model-secrets-offer dummy-source/0 --format json | yq '."dummy-source/0"."relation-info"[0]."relation-id"')
	juju exec --unit dummy-source/0 -- secret-grant "$secret_uri" -r "$relation_id"

	echo "Checking: the secret can be read by the consumer"
	juju switch "model-secrets-consume"
	echo "Checking:  secret-get by URI - consume content"
	check_contains "$(juju_exec_output --unit dummy-sink/0 -- secret-get --label mylabel "$secret_uri")" 'foo: bar'
	echo "Checking:  secret-get by URI - consume content"
	check_contains "$(juju_exec_output --unit dummy-sink/0 -- secret-get --label mylabel)" 'foo: bar'

	echo "Checking: add a new revision and check the consumer is notified"
	# The consumer's registration may have fired a secret-changed hook
	# asynchronously, so wait for the count to settle before recording
	# the baseline.
	juju switch "model-secrets-consume"
	wait_for_hook_count_stable "model-secrets-consume" dummy-sink/0
	secret_changed_before=$(count_secret_changed_hooks "model-secrets-consume" dummy-sink/0)
	juju switch "model-secrets-offer"
	juju exec --unit dummy-source/0 -- secret-set "$secret_uri" foo=bar2
	juju switch "model-secrets-consume"

	echo "Checking: cross model consumer runs secret-changed for the new revision"
	wait_for_secret_changed_hooks "model-secrets-consume" dummy-sink/0 $((secret_changed_before + 1))

	echo "Checking: add a new revision and check consumer can see it"
	check_contains "$(juju_exec_output --unit dummy-sink/0 -- secret-get --label mylabel)" 'foo: bar'
	check_contains "$(juju_exec_output --unit dummy-sink/0 -- secret-get --label mylabel --peek)" 'foo: bar2'
	check_contains "$(juju_exec_output --unit dummy-sink/0 -- secret-get --label mylabel)" 'foo: bar'
	check_contains "$(juju_exec_output --unit dummy-sink/0 -- secret-get --label mylabel --refresh)" 'foo: bar2'
	check_contains "$(juju_exec_output --unit dummy-sink/0 -- secret-get --label mylabel)" 'foo: bar2'

	echo "Checking: suspend relation and check access is lost"
	juju switch "model-secrets-offer"
	juju suspend-relation "$relation_id"
	juju switch "model-secrets-consume"
	check_contains "$(juju_exec_output --unit dummy-sink/0 -- secret-get "$secret_uri" 2>&1)" 'permission denied'
	echo "Checking: resume relation and access is restored"
	juju switch "model-secrets-offer"
	juju resume-relation "$relation_id"
	juju switch "model-secrets-consume"
	check_contains "$(juju_exec_output --unit dummy-sink/0 -- secret-get --label mylabel)" 'foo: bar2'

	echo "Checking: secret-revoke by relation ID"
	juju switch "model-secrets-offer"
	juju exec --unit dummy-source/0 -- secret-revoke "$secret_uri" --relation "$relation_id"
	juju switch "model-secrets-consume"
	check_contains "$(juju_exec_output --unit dummy-sink/0 -- secret-get "$secret_uri" 2>&1)" 'permission denied'
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
