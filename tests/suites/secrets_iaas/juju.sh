check_secrets() {
	juju --show-log deploy juju-qa-dummy-source source
	juju --show-log deploy juju-qa-dummy-sink sink
	juju --show-log integrate sink source
	juju config source token=start # enable relation

	wait_for "active" '.applications["source"] | ."application-status".current'
	wait_for "active" '.applications["sink"] | ."application-status".current' 900
	wait_for "source" "$(idle_condition "source" 0)"
	wait_for "sink" "$(idle_condition "sink" 0)"
	wait_for "active" "$(workload_status "sink" 0).current"

	echo "Apps deployed, creating secrets"
	secret_owned_by_source_0=$(juju exec --unit source/0 -- secret-add --owner unit owned-by=source/0)
	secret_owned_by_source_0_id=${secret_owned_by_source_0##*/}
	secret_owned_by_source=$(juju exec --unit source/0 -- secret-add owned-by=source-app)
	secret_owned_by_source_id=${secret_owned_by_source##*/}

	echo "Set same content again for $secret_owned_by_source."
	juju exec --unit source/0 -- secret-set "$secret_owned_by_source_id" owned-by=source-app

	echo "Checking secret ids"
	check_contains "$(juju exec --unit source/0 -- secret-ids)" "$secret_owned_by_source_id"
	check_contains "$(juju exec --unit source/0 -- secret-ids)" "$secret_owned_by_source_0_id"

	echo "Set a label for the unit owned secret $secret_owned_by_source_0."
	juju exec --unit source/0 -- secret-set "$secret_owned_by_source_0" --label=source_0
	echo "Set a consumer label for the app owned secret $secret_owned_by_source."
	juju exec --unit source/0 -- secret-get "$secret_owned_by_source" --label=source-app

	echo "Checking: secret-get by URI - content"
	check_contains "$(juju exec --unit source/0 -- secret-get "$secret_owned_by_source_0")" 'owned-by: source/0'
	check_contains "$(juju exec --unit source/0 -- secret-get "$secret_owned_by_source")" 'owned-by: source-app'

	echo "Checking: secret-get by URI - metadata"
	check_contains "$(juju exec --unit source/0 -- secret-info-get "$secret_owned_by_source_0" --format json | jq ".${secret_owned_by_source_0_id}.owner")" 'unit'
	check_contains "$(juju exec --unit source/0 -- secret-info-get "$secret_owned_by_source" --format json | jq ".${secret_owned_by_source_id}.owner")" 'application'
	check_contains "$(juju exec --unit source/0 -- secret-info-get "$secret_owned_by_source" --format json | jq ".${secret_owned_by_source_id}.revision")" '1'

	echo "Checking: secret-get by label or consumer label - content"
	check_contains "$(juju exec --unit source/0 -- secret-get --label=source_0)" 'owned-by: source/0'
	check_contains "$(juju exec --unit source/0 -- secret-get --label=source-app)" 'owned-by: source-app'

	echo "Checking: secret-get by label - metadata"
	check_contains "$(juju exec --unit source/0 -- secret-info-get --label=source_0 --format json | jq ".${secret_owned_by_source_0_id}.label")" 'source_0'

	relation_id=$(juju --show-log show-unit source/0 --format json | jq '."source/0"."relation-info"[0]."relation-id"')
	juju exec --unit source/0 -- secret-grant "$secret_owned_by_source_0" -r "$relation_id"
	juju exec --unit source/0 -- secret-grant "$secret_owned_by_source" -r "$relation_id"

	echo "Checking: secret-get by URI - consume content by ID"
	check_contains "$(juju exec --unit sink/0 -- secret-get "$secret_owned_by_source_0" --label=consumer_label_secret_owned_by_source_0)" 'owned-by: source/0'
	check_contains "$(juju exec --unit sink/0 -- secret-get "$secret_owned_by_source" --label=consumer_label_secret_owned_by_source)" 'owned-by: source-app'

	echo "Checking: secret-get by URI - consume content by label"
	check_contains "$(juju exec --unit sink/0 -- secret-get --label=consumer_label_secret_owned_by_source_0)" 'owned-by: source/0'
	check_contains "$(juju exec --unit sink/0 -- secret-get --label=consumer_label_secret_owned_by_source)" 'owned-by: source-app'

	echo "Set different content for $secret_owned_by_source."
	juju exec --unit source/0 -- secret-set "$secret_owned_by_source_id" foo=bar
	check_contains "$(juju exec --unit source/0 -- secret-info-get "$secret_owned_by_source" --format json | jq ".${secret_owned_by_source_id}.revision")" '2'
	check_contains "$(juju exec --unit source/0 -- secret-get --refresh "$secret_owned_by_source")" 'foo: bar'

	echo "Checking: secret-revoke by relation ID"
	juju exec --unit source/0 -- secret-revoke "$secret_owned_by_source" --relation "$relation_id"
	check_contains "$(juju exec --unit sink/0 -- secret-get "$secret_owned_by_source" 2>&1)" 'is not allowed to read this secret'

	echo "Checking: secret-revoke by app name"
	juju exec --unit source/0 -- secret-revoke "$secret_owned_by_source_0" --app sink
	check_contains "$(juju exec --unit sink/0 -- secret-get "$secret_owned_by_source_0" 2>&1)" 'is not allowed to read this secret'

	echo "Checking: secret-remove"
	juju exec --unit source/0 -- secret-remove "$secret_owned_by_source_0"
	check_contains "$(juju exec --unit source/0 -- secret-get "$secret_owned_by_source_0" 2>&1)" 'not found'
	juju exec --unit source/0 -- secret-remove "$secret_owned_by_source"
	check_contains "$(juju exec --unit source/0 -- secret-get "$secret_owned_by_source" 2>&1)" 'not found'
}

run_user_secrets() {
	echo

	model_name=${1}

	app_name='test-user-secrets'
	juju --show-log deploy juju-qa-test "$app_name"
	
	wait_for "active" '.applications["test-user-secrets"] | ."application-status".current'

	# first test the creation of a large secret which encodes to approx 1MB in size.
	echo "data: $(cat /dev/zero | tr '\0' A | head -c 749500)" >"${TEST_DIR}/secret.txt"
	secret_uri=$(juju add-secret big --file "${TEST_DIR}/secret.txt")
	secret_short_uri=${secret_uri##*:}
	check_contains "$(juju show-secret big --reveal | yq ".${secret_short_uri}.content.data" | grep -o A | wc -l)" 749500
	juju --show-log remove-secret big

	# test user secret revisions and grants.
	secret_uri=$(juju --show-log add-secret mysecret owned-by="$model_name-1" --info "this is a user secret")
	secret_short_uri=${secret_uri##*:}

	check_contains "$(juju --show-log show-secret mysecret --revisions | yq ".${secret_short_uri}.description")" 'this is a user secret'

	# set same content again for revision 1.
	juju --show-log update-secret "$secret_uri" owned-by="$model_name-1"
	check_contains "$(juju --show-log show-secret "$secret_uri" --revisions | yq ".${secret_short_uri}.description")" 'this is a user secret'
	check_contains "$(juju --show-log show-secret "$secret_uri" | yq ".${secret_short_uri}.revision")" '1'

	# create a new revision 2.
	juju --show-log update-secret "$secret_uri" --info info owned-by="$model_name-2"
	check_contains "$(juju --show-log show-secret "$secret_uri" --revisions | yq ".${secret_short_uri}.description")" 'info'

	# grant secret to the app, and now the application can access the revision 2.
	check_contains "$(juju exec --unit "$app_name"/0 -- secret-get "$secret_uri" 2>&1)" 'is not allowed to read this secret'
	juju --show-log grant-secret mysecret "$app_name"
	check_contains "$(juju exec --unit "$app_name/0" -- secret-get $secret_short_uri)" "owned-by: $model_name-2"

	# create a new revision 3.
	juju --show-log update-secret "$secret_uri" owned-by="$model_name-3"

	check_contains "$(juju --show-log show-secret $secret_uri --revisions | yq .${secret_short_uri}.revision)" '3'
	check_contains "$(juju --show-log show-secret $secret_uri --revisions | yq .${secret_short_uri}.owner)" "<model>"
	check_contains "$(juju --show-log show-secret $secret_uri --revisions | yq .${secret_short_uri}.description)" 'info'
	check_num_secret_revisions "$secret_uri" "$secret_short_uri" 3

	check_contains "$(juju --show-log show-secret $secret_uri --reveal --revision 1 | yq .${secret_short_uri}.content)" "owned-by: $model_name-1"
	check_contains "$(juju --show-log show-secret $secret_uri --reveal --revision 2 | yq .${secret_short_uri}.content)" "owned-by: $model_name-2"
	check_contains "$(juju --show-log show-secret $secret_uri --reveal --revision 3 | yq .${secret_short_uri}.content)" "owned-by: $model_name-3"

	# turn on --auto-prune
	juju --show-log update-secret mysecret --auto-prune=true

	# revision 1 should be pruned.
	# revision 2 is still been used by the app, so it should not be pruned.
	# revision 3 is the latest revision, so it should not be pruned.
	check_num_secret_revisions "$secret_uri" "$secret_short_uri" 2
	check_contains "$(juju --show-log show-secret $secret_uri --reveal --revision 2 | yq .${secret_short_uri}.content)" "owned-by: $model_name-2"
	check_contains "$(juju --show-log show-secret $secret_uri --reveal --revision 3 | yq .${secret_short_uri}.content)" "owned-by: $model_name-3"

	check_contains "$(juju exec --unit "$app_name/0" -- secret-get $secret_short_uri --peek)" "owned-by: $model_name-3"
	check_contains "$(juju exec --unit "$app_name/0" -- secret-get $secret_short_uri --refresh)" "owned-by: $model_name-3"
	check_contains "$(juju exec --unit "$app_name/0" -- secret-get $secret_short_uri)" "owned-by: $model_name-3"

	# revision 2 should be pruned.
	# revision 3 is the latest revision, so it should not be pruned.
	check_num_secret_revisions "$secret_uri" "$secret_short_uri" 1
	check_contains "$(juju --show-log show-secret $secret_uri --reveal --revision 3 | yq .${secret_short_uri}.content)" "owned-by: $model_name-3"

	juju --show-log revoke-secret mysecret "$app_name"
	check_contains "$(juju exec --unit "$app_name"/0 -- secret-get "$secret_uri" 2>&1)" 'is not allowed to read this secret'

	juju --show-log remove-secret mysecret
	check_contains "$(juju --show-log secrets --format yaml | yq length)" '0'
}

run_secrets_juju() {
	echo

	model_name='model-secrets-juju'
	add_model "$model_name"
	check_secrets
	run_user_secrets "$model_name"
	destroy_model "$model_name"
}

test_secrets_juju() {
	if [ "$(skip 'test_secrets_juju')" ]; then
		echo "==> TEST SKIPPED: test_secrets_juju"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_secrets_juju"
	)
}

obsolete_secret_revisions() {
	local secret_short_uri
	secret_short_uri=${1}

	out=$(
		juju ssh ubuntu-lite/0 sh <<EOF
. /etc/profile.d/juju-introspection.sh
juju_engine_report | sed 1d | yq '..style="flow" | .manifolds.deployer.report.units.workers.ubuntu-lite/0.report.manifolds.uniter.report.secrets.obsolete-revisions."'"${secret_short_uri}"'"'
EOF
	)
	echo "${out}"
}

run_obsolete_revisions() {
	echo

	model_name=${1}

	juju --show-log deploy jameinel-ubuntu-lite
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"
	juju ssh ubuntu-lite/0 "sudo snap install yq"

	secret_uri=$(juju --show-log exec -u ubuntu-lite/0 -- secret-add foo=bar)
	secret_short_uri="secret:${secret_uri##*/}"

	# Create 10 new revisions, so we'll have 11 in total. 1-10 will be obsolete.
	for i in $(seq 10); do juju --show-log exec --unit ubuntu-lite/0 -- secret-set "$secret_uri" foo="$i"; done

	# Check that the secret-remove hook is run for the 10 obsolete revisions.
	attempt=0
	while true; do
		num_hooks=$(juju show-status-log ubuntu-lite/0 --format yaml -n 100 | yq -o json | jq -r '[.[] | select(.message != null) | select(.message | contains("running secret-remove hook for '"${secret_short_uri}"'"))] | length')
		if [ "$num_hooks" -eq 10 ]; then
			break
		fi
		attempt=$((attempt + 1))
		if [ $attempt -eq 10 ]; then
			# shellcheck disable=SC2046
			echo $(red "expected 10 secret-remove hooks, got $num_hooks")
			exit 1
		fi
		sleep 2
	done

	# Check that the unit state contains the 10 obsolete revisions.
	echo "Checking initial obsolete revisions 1..10"
	attempt=0
	while true; do
		obsolete=$(obsolete_secret_revisions "${secret_short_uri}")
		if [ "$obsolete" == "[1, 2, 3, 4, 5, 6, 7, 8, 9, 10]" ]; then
			break
		fi
		attempt=$((attempt + 1))
		if [ $attempt -eq 10 ]; then
			# shellcheck disable=SC2046
			echo $(red "expected [1, 2, 3, 4, 5, 6, 7, 8, 9, 10] initial obsolete revisions, got $obsolete")
			exit 1
		fi
		sleep 2
	done

	# Remove a single revision.
	juju --show-log exec --unit ubuntu-lite/0 -- secret-remove "$secret_uri" --revision 6

	# Check that the unit state has the deleted revision removed.
	echo "Checking revision 6 has been removed from the obsolete revisions"
	attempt=0
	while true; do
		obsolete=$(obsolete_secret_revisions "${secret_short_uri}")
		if [ "$obsolete" == "[1, 2, 3, 4, 5, 7, 8, 9, 10]" ]; then
			break
		fi
		attempt=$((attempt + 1))
		if [ $attempt -eq 10 ]; then
			# shellcheck disable=SC2046
			echo $(red "expected [1, 2, 3, 4, 5, 7, 8, 9, 10] obsolete revisions, got $obsolete")
			exit 1
		fi
		sleep 2
	done

	# Delete the entire secret.
	juju --show-log exec --unit ubuntu-lite/0 -- secret-remove "$secret_uri"

	# Check that all the obsolete revisions are removed from unit state.
	echo "Checking all obsolete revision are removed when the secret is deleted"
	attempt=0
	while true; do
		obsolete=$(obsolete_secret_revisions "${secret_short_uri}")
		if [ $obsolete == null ]; then
			break
		fi
		attempt=$((attempt + 1))
		if [ $attempt -eq 10 ]; then
			# shellcheck disable=SC2046
			echo $(red "expected no obsolete revisions, got $obsolete")
			exit 1
		fi
		sleep 2
	done
}

test_obsolete_revisions() {
	if [ "$(skip 'test_obsolete_revisions')" ]; then
		echo "==> TEST SKIPPED: test_obsolete_revisions"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		model_name='model-secrets-obsolete'
		add_model "$model_name"
		run_obsolete_revisions "$model_name"
		destroy_model "$model_name"
	)
}
