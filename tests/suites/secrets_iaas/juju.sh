check_secrets() {
	# Optional expected backend name (defaults to "internal")
	expected_backend=${1:-internal}

	juju --show-log deploy juju-qa-dummy-source
	juju --show-log deploy juju-qa-dummy-sink
	juju --show-log integrate dummy-sink dummy-source
	juju config dummy-source token=foo

	wait_for "active" '.applications["dummy-source"] | ."application-status".current'
	wait_for "active" '.applications["dummy-sink"] | ."application-status".current' 900
	wait_for "dummy-source" "$(idle_condition "dummy-source" 0)"
	wait_for "dummy-sink" "$(idle_condition "dummy-sink" 0)"
	wait_for "active" "$(workload_status "dummy-source" 0).current"
	wait_for "active" "$(workload_status "dummy-sink" 0).current"

	echo "Apps deployed, creating secrets"
	secret_owned_by_dummy_source_0=$(juju_exec_output --unit dummy-source/0 -- secret-add --owner unit owned-by=dummy-source/0)
	secret_owned_by_dummy_source_0_id=${secret_owned_by_dummy_source_0##*/}
	secret_owned_by_dummy_source=$(juju_exec_output --unit dummy-source/0 -- secret-add owned-by=dummy-source-app)
	secret_owned_by_dummy_source_id=${secret_owned_by_dummy_source##*/}

	echo "Checking secrets' backend name with juju secrets --revisions"
	check_contains "$(juju secrets --revisions --format yaml | yq -r ".${secret_owned_by_dummy_source_id}.revisions[0].backend")" "${expected_backend}"
	check_contains "$(juju secrets --revisions --format yaml | yq -r ".${secret_owned_by_dummy_source_0_id}.revisions[0].backend")" "${expected_backend}"

	echo "Checking secrets' backend name with juju secrets --owner --revisions"
	check_contains "$(juju secrets --owner application-dummy-source --revisions --format yaml | yq -r ".${secret_owned_by_dummy_source_id}.revisions[0].backend")" "${expected_backend}"
	check_contains "$(juju secrets --owner unit-dummy-source-0 --revisions --format yaml | yq -r ".${secret_owned_by_dummy_source_0_id}.revisions[0].backend")" "${expected_backend}"

	echo "Checking secrets' backend name with juju show-secret --revisions"
	check_contains "$(juju --show-log show-secret "$secret_owned_by_dummy_source_id" --revisions --format yaml | yq -r ".${secret_owned_by_dummy_source_id}.revisions[0].backend")" "${expected_backend}"
	check_contains "$(juju --show-log show-secret "$secret_owned_by_dummy_source_0_id" --revisions --format yaml | yq -r ".${secret_owned_by_dummy_source_0_id}.revisions[0].backend")" "${expected_backend}"

	echo "Set same content again for $secret_owned_by_dummy_source."
	juju exec --unit dummy-source/0 -- secret-set "$secret_owned_by_dummy_source_id" owned-by=dummy-source-app

	echo "Checking secret ids"
	check_contains "$(juju_exec_output --unit dummy-source/0 -- secret-ids)" "$secret_owned_by_dummy_source_id"
	check_contains "$(juju_exec_output --unit dummy-source/0 -- secret-ids)" "$secret_owned_by_dummy_source_0_id"

	echo "Set a label for the unit owned secret $secret_owned_by_dummy_source_0."
	juju exec --unit dummy-source/0 -- secret-set "$secret_owned_by_dummy_source_0" --label=dummy-source_0
	echo "Set a consumer label for the app owned secret $secret_owned_by_dummy_source."
	juju exec --unit dummy-source/0 -- secret-get "$secret_owned_by_dummy_source" --label=dummy-source-app

	echo "Checking: secret-get by URI - content"
	check_contains "$(juju_exec_output --unit dummy-source/0 -- secret-get "$secret_owned_by_dummy_source_0")" 'owned-by: dummy-source/0'
	check_contains "$(juju_exec_output --unit dummy-source/0 -- secret-get "$secret_owned_by_dummy_source")" 'owned-by: dummy-source-app'

	echo "Checking: secret-get by URI - metadata"
	check_contains "$(juju_exec_output --unit dummy-source/0 -- secret-info-get "$secret_owned_by_dummy_source_0" --format json | yq ".${secret_owned_by_dummy_source_0_id}.owner")" 'unit'
	check_contains "$(juju_exec_output --unit dummy-source/0 -- secret-info-get "$secret_owned_by_dummy_source" --format json | yq ".${secret_owned_by_dummy_source_id}.owner")" 'application'
	check_contains "$(juju_exec_output --unit dummy-source/0 -- secret-info-get "$secret_owned_by_dummy_source" --format json | yq ".${secret_owned_by_dummy_source_id}.revision")" '1'

	echo "Checking: secret-get by label or consumer label - content"
	check_contains "$(juju_exec_output --unit dummy-source/0 -- secret-get --label=dummy-source_0)" 'owned-by: dummy-source/0'
	check_contains "$(juju_exec_output --unit dummy-source/0 -- secret-get --label=dummy-source-app)" 'owned-by: dummy-source-app'

	echo "Checking: secret-get by label - metadata"
	check_contains "$(juju_exec_output --unit dummy-source/0 -- secret-info-get --label=dummy-source_0 --format json | yq ".${secret_owned_by_dummy_source_0_id}.label")" 'dummy-source_0'

	relation_id=$(juju --show-log show-unit dummy-source/0 --format json | yq '."dummy-source/0"."relation-info"[0]."relation-id"')
	juju exec --unit dummy-source/0 -- secret-grant "$secret_owned_by_dummy_source_0" -r "$relation_id"
	juju exec --unit dummy-source/0 -- secret-grant "$secret_owned_by_dummy_source" -r "$relation_id"

	echo "Checking: secret-get by label - refresh with pending updates"
	another_secret_owned_by_dummy_source=$(juju_exec_output --unit dummy-source/0 -- secret-add value=1 --label=mysecret)
	check_contains "$(juju_exec_output --unit dummy-source/0 -- "secret-set ${another_secret_owned_by_dummy_source} value=2; secret-get ${another_secret_owned_by_dummy_source} --refresh")" "2"
	check_contains "$(juju_exec_output --unit dummy-source/0 -- "secret-set ${another_secret_owned_by_dummy_source} value=3; secret-get --label=mysecret --refresh")" "3"
	check_contains "$(juju_exec_output --unit dummy-source/0 -- "secret-get --label=mysecret")" "3"
	juju exec --unit dummy-source/0 -- "secret-set ${another_secret_owned_by_dummy_source} value=4"
	check_contains "$(juju_exec_output --unit dummy-source/0 -- secret-get --label=mysecret --refresh)" "4"

	echo "Checking: secret-get by URI - consume content by ID"
	check_contains "$(juju_exec_output --unit dummy-sink/0 -- secret-get "$secret_owned_by_dummy_source_0" --label=consumer_label_secret_owned_by_dummy_source_0)" 'owned-by: dummy-source/0'
	check_contains "$(juju_exec_output --unit dummy-sink/0 -- secret-get "$secret_owned_by_dummy_source" --label=consumer_label_secret_owned_by_dummy_source)" 'owned-by: dummy-source-app'

	echo "Checking: secret-get by URI - consume content by label"
	check_contains "$(juju_exec_output --unit dummy-sink/0 -- secret-get --label=consumer_label_secret_owned_by_dummy_source_0)" 'owned-by: dummy-source/0'
	check_contains "$(juju_exec_output --unit dummy-sink/0 -- secret-get --label=consumer_label_secret_owned_by_dummy_source)" 'owned-by: dummy-source-app'

	echo "Set different content for $secret_owned_by_dummy_source."
	juju exec --unit dummy-source/0 -- secret-set "$secret_owned_by_dummy_source_id" foo=bar
	check_contains "$(juju_exec_output --unit dummy-source/0 -- secret-info-get "$secret_owned_by_dummy_source" --format json | yq ".${secret_owned_by_dummy_source_id}.revision")" '2'
	check_contains "$(juju_exec_output --unit dummy-source/0 -- secret-get --refresh "$secret_owned_by_dummy_source")" 'foo: bar'

	echo "Checking: secret-revoke by relation ID"
	juju exec --unit dummy-source/0 -- secret-revoke "$secret_owned_by_dummy_source" --relation "$relation_id"
	check_contains "$(juju_exec_output --unit dummy-sink/0 -- secret-get "$secret_owned_by_dummy_source" 2>&1)" 'is not allowed to read this secret'

	echo "Checking: secret-revoke by app name"
	juju exec --unit dummy-source/0 -- secret-revoke "$secret_owned_by_dummy_source_0" --app dummy-sink
	check_contains "$(juju_exec_output --unit dummy-sink/0 -- secret-get "$secret_owned_by_dummy_source_0" 2>&1)" 'is not allowed to read this secret'

	echo "Checking secret rotate"
	juju exec --unit dummy-source/0 -- secret-set "$secret_owned_by_dummy_source_0" --rotate daily
	check_contains "$(juju show-secret "$secret_owned_by_dummy_source_0" --format json | yq ".[].rotation")" "daily"
	original_rotate_time="$(juju show-secret "$secret_owned_by_dummy_source_0" --format json | yq ".[].rotates")"
	# We set a new rotate time into the future and we need to retain
	# the current next rotate time.
	juju exec --unit dummy-source/0 -- secret-set "$secret_owned_by_dummy_source_0" --rotate monthly
	check_contains "$(juju show-secret "$secret_owned_by_dummy_source_0" --format json | yq ".[].rotation")" "monthly"
	next_rotate_time="$(juju show-secret "$secret_owned_by_dummy_source_0" --format json | yq ".[].rotates")"
	if [[ $original_rotate_time != "$next_rotate_time" ]]; then
		echo "secret next rotate time was updated in error"
		exit 1
	fi
	# We set a new rotate time sooner than the current rotate time so we need to
	# update the next rotate time.
	juju exec --unit dummy-source/0 -- secret-set "$secret_owned_by_dummy_source_0" --rotate hourly
	check_contains "$(juju show-secret "$secret_owned_by_dummy_source_0" --format json | yq ".[].rotation")" "hourly"
	next_rotate_time="$(juju show-secret "$secret_owned_by_dummy_source_0" --format json | yq ".[].rotates")"
	if [[ $original_rotate_time == "$next_rotate_time" ]]; then
		echo "secret next rotate time was not updated"
		exit 1
	fi

	echo "Checking: secret-remove"
	juju exec --unit dummy-source/0 -- secret-remove "$secret_owned_by_dummy_source_0"
	check_contains "$(juju_exec_output --unit dummy-source/0 -- secret-get "$secret_owned_by_dummy_source_0" 2>&1)" 'not found'
	juju exec --unit dummy-source/0 -- secret-remove "$secret_owned_by_dummy_source"
	check_contains "$(juju_exec_output --unit dummy-source/0 -- secret-get "$secret_owned_by_dummy_source" 2>&1)" 'not found'
	juju exec --unit dummy-source/0 -- secret-remove "$another_secret_owned_by_dummy_source"
	check_contains "$(juju_exec_output --unit dummy-source/0 -- secret-get "$another_secret_owned_by_dummy_source" 2>&1)" 'not found'
}

run_user_secrets() {
	echo

	model_name=${1}

	app_name='dummy-user-secrets'
	juju --show-log deploy juju-qa-test "$app_name"

	wait_for "active" '.applications["dummy-user-secrets"] | ."application-status".current'

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
	check_contains "$(juju_exec_output --unit "$app_name"/0 -- secret-get "$secret_uri" 2>&1)" 'is not allowed to read this secret'
	juju --show-log grant-secret mysecret "$app_name"
	check_contains "$(juju_exec_output --unit "$app_name/0" -- secret-get $secret_short_uri)" "owned-by: $model_name-2"

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

	check_contains "$(juju_exec_output --unit "$app_name/0" -- secret-get $secret_short_uri --peek)" "owned-by: $model_name-3"
	check_contains "$(juju_exec_output --unit "$app_name/0" -- secret-get $secret_short_uri --refresh)" "owned-by: $model_name-3"
	check_contains "$(juju_exec_output --unit "$app_name/0" -- secret-get $secret_short_uri)" "owned-by: $model_name-3"

	# revision 2 should be pruned.
	# revision 3 is the latest revision, so it should not be pruned.
	check_num_secret_revisions "$secret_uri" "$secret_short_uri" 1
	check_contains "$(juju --show-log show-secret $secret_uri --reveal --revision 3 | yq .${secret_short_uri}.content)" "owned-by: $model_name-3"

	juju --show-log revoke-secret mysecret "$app_name"
	check_contains "$(juju_exec_output --unit "$app_name"/0 -- secret-get "$secret_uri" 2>&1)" 'is not allowed to read this secret'

	juju --show-log remove-secret mysecret
	check_num_secrets 0
}

run_secrets_juju() {
	echo

	model_name='model-secrets-juju'
	add_model "$model_name"
	check_secrets "internal"
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
	local secret_id
	secret_id=${1}

	yaml_out=$(
juju ssh juju-qa-test/0 sh <<EOF
. /etc/profile.d/juju-introspection.sh
juju_engine_report
EOF
	)
	 out=$(echo "${yaml_out}" | sed 1d | yq "..style=\"flow\" | .manifolds.deployer.report.handler.units.workers.\"juju-qa-test/0\".report.manifolds.uniter.report.secrets.\"obsolete-revisions\".\"${secret_id}\"")
	 echo "${out}"
}

run_obsolete_revisions() {
	echo

	model_name=${1}

	juju --show-log deploy juju-qa-test
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"

	secret_uri=$(juju_exec_output -u juju-qa-test/0 -- secret-add foo=bar)
	# Extract bare secret id (without "secret:" prefix) for matching logs and keep prefixed form for engine report keys.
	secret_id=${secret_uri##*/}
	secret_short_uri="secret:${secret_id}"

	# Create 10 new revisions, so we'll have 11 in total. 1-10 will be obsolete.
	for i in $(seq 10); do juju --show-log exec --unit juju-qa-test/0 -- secret-set "$secret_uri" foo="$i"; done

	# Check that the secret-remove hook is run for the 10 obsolete revisions.
	attempt=0
	while true; do
		num_hooks=$(juju show-status-log juju-qa-test/0 --format json -n 100 | yq -r "[.[] | select(.message != null) | select(.message | contains(\"running secret-remove hook\") and .message | contains(\"${secret_id}\"))] | length")
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
		obsolete=$(obsolete_secret_revisions "${secret_id}")
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
	juju --show-log exec --unit juju-qa-test/0 -- secret-remove "$secret_uri" --revision 6

	# Check that the unit state has the deleted revision removed.
	echo "Checking revision 6 has been removed from the obsolete revisions"
	attempt=0
	while true; do
		obsolete=$(obsolete_secret_revisions "${secret_id}")
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
	juju --show-log exec --unit juju-qa-test/0 -- secret-remove "$secret_uri"

	# Check that all the obsolete revisions are removed from unit state.
	echo "Checking all obsolete revision are removed when the secret is deleted"
	attempt=0
	while true; do
		obsolete=$(obsolete_secret_revisions "${secret_id}")
		if [ "$obsolete" == null ]; then
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

# Regression test: a non-leader unit must be able to create and delete
# its own unit-owned secrets without hitting a "lease not held" error.
run_secret_nonleader_unit_owned() {
	echo

	juju --show-log deploy juju-qa-dummy-source -n 2
	juju --show-log deploy juju-qa-dummy-sink
	juju --show-log integrate dummy-sink dummy-source
	wait_for "dummy-source" "$(idle_condition "dummy-source" 0)"
	wait_for "dummy-source" "$(idle_condition "dummy-source" 1)"
	wait_for "dummy-sink" "$(idle_condition "dummy-sink" 0)"

	# Identify the non-leader unit.
	if juju exec --unit dummy-source/0 -- is-leader 2>/dev/null | grep -q true; then
		non_leader="dummy-source/1"
	else
		non_leader="dummy-source/0"
	fi
	echo "Non-leader unit: $non_leader"

	echo "Checking: non-leader can create and remove its own unit-owned secret"
	secret_uri=$(juju exec --unit "$non_leader" -- secret-add --owner unit nonleader=secret)
	juju exec --unit "$non_leader" -- secret-remove "$secret_uri"
	check_contains "$(juju exec --unit "$non_leader" -- secret-get "$secret_uri" 2>&1)" 'not found'

	echo "Checking: non-leader can grant its own unit-owned secret"
	secret_uri=$(juju exec --unit "$non_leader" -- secret-add --owner unit grantme=value)
	relation_id=$(juju --show-log show-unit "$non_leader" --format json | yq ".\"${non_leader}\".\"relation-info\"[0].\"relation-id\"")
	juju exec --unit "$non_leader" -- secret-grant "$secret_uri" -r "$relation_id"

	echo "Checking: consumer can read the granted secret"
	check_contains "$(juju exec --unit dummy-sink/0 -- secret-get "$secret_uri")" 'grantme: value'

	echo "Checking: non-leader can revoke its own unit-owned secret"
	juju exec --unit "$non_leader" -- secret-revoke "$secret_uri" --app dummy-sink
	check_contains "$(juju exec --unit dummy-sink/0 -- secret-get "$secret_uri" 2>&1)" 'is not allowed to read this secret'

	echo "Cleaning up non-leader secret"
	juju exec --unit "$non_leader" -- secret-remove "$secret_uri"
}

test_secret_nonleader_unit_owned() {
	if [ "$(skip 'test_secret_nonleader_unit_owned')" ]; then
		echo "==> TEST SKIPPED: test_secret_nonleader_unit_owned"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		model_name='model-secrets-nonleader'
		add_model "$model_name"
		run_secret_nonleader_unit_owned "$model_name"
		destroy_model "$model_name"
	)
}

# run_track_latest_revision verifies that CommitHookChanges correctly updates
# the consumer tracking record to the latest revision when a unit updates a
# secret and refreshes in the same hook execution.
#
# Sequence:
#   1. secret-add (rev 1) + secret-get  -> consumer record at current_revision=1
#   2. secret-set (rev 2) + secret-get --refresh in ONE juju exec invocation
#      -> pendingTrackLatest fires -> CommitHookChanges -> trackSecrets
#      -> consumer record updated to current_revision=2
#      -> markSecretRevisionsObsolete inserts rev 1 -> WatchObsolete fires
#      -> rev 1 appears in the engine report obsolete-revisions
#   3. secret-get (no --refresh) in a fresh hook -> must return rev 2 value;
#      returning rev 1 value would mean tracking was not updated.
run_track_latest_revision() {
	echo

	# Step 1: create secret at rev 1 and read it.
	# Reading establishes a consumer record at current_revision=1.
	secret_uri=$(juju exec --unit juju-qa-test/0 -- secret-add val=one)
	secret_id=${secret_uri##*/}
	check_contains "$(juju exec --unit juju-qa-test/0 -- secret-get "$secret_uri")" 'val: one'

	# Step 2: update to rev 2 and refresh tracking in a single hook execution
	# (single CommitHookChanges call). trackSecrets must update the consumer
	# record to current_revision=2 inside the DB transaction, and
	# markSecretRevisionsObsolete must insert rev 1 into secret_revision_obsolete.
	check_contains "$(juju exec --unit juju-qa-test/0 \
		-- "secret-set $secret_uri val=two; secret-get $secret_uri --refresh")" 'val: two'

	# Revision 1 must appear in the engine report's obsolete-revisions map.
	# WatchObsolete fires asynchronously after the DB write, so retry briefly.
	echo "Checking: revision 1 is marked obsolete after CommitHookChanges"
	attempt=0
	while true; do
		obsolete=$(obsolete_secret_revisions "${secret_id}")
		if [ "$obsolete" = "[1]" ]; then break; fi
		attempt=$((attempt + 1))
		if [ $attempt -eq 10 ]; then
			# shellcheck disable=SC2046
			echo $(red "expected '[1]' in obsolete-revisions, got '$obsolete'")
			exit 1
		fi
		sleep 2
	done

	# Step 3: fresh hook, no --refresh flag. GetConsumedRevision returns
	# current_revision=2 from the consumer record, so the unit sees rev 2.
	# If trackSecrets did not run, current_revision would still be 1 and
	# this check would return 'val: one' instead.
	echo "Checking: consumer record tracks revision 2 after CommitHookChanges"
	check_contains "$(juju exec --unit juju-qa-test/0 -- secret-get "$secret_uri")" 'val: two'
}

# run_atomic_secret_create verifies that multiple secret creations through
# CommitHookChanges (JUJU-9034) commit atomically in a single hook execution.
# Two secrets created in one hook must both be readable immediately.
run_atomic_secret_create() {
	echo

	# Create two secrets in one hook execution. Both must appear in
	# secret_ids and be readable immediately, proving atomic commit.
	echo "Checking: multiple secret-add in one hook commits atomically"
	output=$(juju exec --unit juju-qa-test/0 -- \
		"secret_uri1=\$(secret-add val=first --label=create-one); echo \$secret_uri1; secret_uri2=\$(secret-add val=second --label=create-two); echo \$secret_uri2")

	secret_uri1=$(echo "$output" | head -1)
	secret_uri2=$(echo "$output" | tail -1)
	secret_id1=${secret_uri1##*/}
	secret_id2=${secret_uri2##*/}

	# Verify both secrets exist and are readable.
	check_contains "$(juju exec --unit juju-qa-test/0 -- secret-get "$secret_uri1")" 'val: first'
	check_contains "$(juju exec --unit juju-qa-test/0 -- secret-get "$secret_uri2")" 'val: second'
	check_contains "$(juju exec --unit juju-qa-test/0 -- secret-info-get "$secret_uri1" --format json | yq ".${secret_id1}.label")" 'create-one'
	check_contains "$(juju exec --unit juju-qa-test/0 -- secret-info-get "$secret_uri2" --format json | yq ".${secret_id2}.label")" 'create-two'

	# Verify both appear in secret-ids output.
	secret_ids=$(juju exec --unit juju-qa-test/0 -- secret-ids)
	check_contains "$secret_ids" "$secret_id1"
	check_contains "$secret_ids" "$secret_id2"

	echo "Cleaning up atomic create secrets"
	juju exec --unit juju-qa-test/0 -- secret-remove "$secret_uri1"
	juju exec --unit juju-qa-test/0 -- secret-remove "$secret_uri2"
}

# run_atomic_secret_update verifies that a unit-owned secret update applied
# through CommitHookChanges (JUJU-9962) commits atomically with other hook
# changes. Secret updates now flow through the CommitHookChanges domain
# transaction rather than a dedicated facade method.
#
# Scenarios:
#   1. content + metadata update in a single hook -> new revision persisted,
#      label/rotate applied, old revision marked obsolete.
run_atomic_secret_update() {
	echo

	# Step 1: create a secret at rev 1.
	secret_uri=$(juju exec --unit juju-qa-test/0 -- secret-add val=one)
	secret_id=${secret_uri##*/}
	check_contains "$(juju exec --unit juju-qa-test/0 -- secret-get "$secret_uri")" 'val: one'

	# Step 2: update content and metadata in a single hook execution (one
	# CommitHookChanges call). The content update creates rev 2 and the
	# metadata changes (label, rotate) must be applied in the same
	# transaction.
	echo "Checking: content + metadata update in one hook"
	check_contains "$(juju exec --unit juju-qa-test/0 \
		-- "secret-set $secret_uri val=two --label=mylabel --rotate=daily; secret-get $secret_uri --refresh")" 'val: two'

	# The metadata changes must have been persisted by the same transaction.
	check_contains "$(juju exec --unit juju-qa-test/0 -- secret-info-get "$secret_uri" --format json | yq ".${secret_id}.label")" 'mylabel'
	check_contains "$(juju exec --unit juju-qa-test/0 -- secret-info-get "$secret_uri" --format json | yq ".${secret_id}.revision")" '2'
	check_contains "$(juju show-secret "$secret_uri" --format json | yq ".[].rotation")" 'daily'

	# Fresh hook with no --refresh: the consumer record must already track
	# rev 2, proving the update was committed.
	check_contains "$(juju exec --unit juju-qa-test/0 -- secret-get "$secret_uri")" 'val: two'

	# Rev 1 must be marked obsolete after the update.
	echo "Checking: revision 1 is marked obsolete after the update"
	attempt=0
	while true; do
		obsolete=$(obsolete_secret_revisions "${secret_id}")
		if [ "$obsolete" = "[1]" ]; then break; fi
		attempt=$((attempt + 1))
		if [ $attempt -eq 10 ]; then
			# shellcheck disable=SC2046
			echo $(red "expected '[1]' in obsolete-revisions, got '$obsolete'")
			exit 1
		fi
		sleep 2
	done

	echo "Cleaning up atomic update secret"
	juju exec --unit juju-qa-test/0 -- secret-remove "$secret_uri"
}

# run_atomic_secret_update_grant verifies that a content update and a grant
# applied in the same hook execution commit together through
# CommitHookChanges (JUJU-9962): the consumer must be able to read the
# updated revision. Uses the dummy-source/dummy-sink pair which has a
# compatible relation for granting.
run_atomic_secret_update_grant() {
	echo

	secret_uri=$(juju exec --unit dummy-source/0 -- secret-add --owner unit val=one)
	relation_id=$(juju --show-log show-unit dummy-source/0 --format json | yq '."dummy-source/0"."relation-info"[0]."relation-id"')

	# Update the content and grant to the consumer in a single hook. The
	# secret update and the grant must commit together so the consumer can
	# read the new revision.
	echo "Checking: content update + grant in one hook"
	juju exec --unit dummy-source/0 -- "secret-set $secret_uri val=two; secret-grant $secret_uri -r $relation_id"
	check_contains "$(juju exec --unit dummy-sink/0 -- secret-get "$secret_uri")" 'val: two'

	echo "Cleaning up atomic update grant secret"
	juju exec --unit dummy-source/0 -- secret-remove "$secret_uri"
}

# run_checksum_deduplication_no_leak verifies that updating a secret with
# the same checksum does not leak backend references. When the content
# checksum matches the current revision, no new revision is created and
# the backend reference should remain consistent.
#
# Regression test for JUJU-9962: backend reference leak on same-checksum updates.
run_checksum_deduplication_no_leak() {
	echo

	secret_uri=$(juju exec --unit juju-qa-test/0 -- secret-add val=original)
	secret_id=${secret_uri##*/}

	initial_revs=$(juju show-secret "$secret_uri" --revisions --format yaml | yq ".${secret_id}.revisions | length")
	echo "Initial revisions: $initial_revs"

	initial_backend=$(juju secrets --revisions --format yaml | yq -r ".${secret_id}.revisions[0].backend")
	echo "Initial backend: $initial_backend"

	juju exec --unit juju-qa-test/0 -- secret-set "$secret_uri" val=original

	check_num_secret_revisions "$secret_uri" "$secret_id" "$initial_revs"

	after_same_checksum_backend=$(juju secrets --revisions --format yaml | yq -r ".${secret_id}.revisions[0].backend")
	echo "Backend after same-checksum update: $after_same_checksum_backend"
	check_contains "$after_same_checksum_backend" "$initial_backend"

	juju exec --unit juju-qa-test/0 -- secret-set "$secret_uri" val=modified

	expected_revs=$((initial_revs + 1))
	check_num_secret_revisions "$secret_uri" "$secret_id" "$expected_revs"

	new_rev_backend=$(juju secrets --revisions --format yaml | yq -r ".${secret_id}.revisions[1].backend")
	echo "Backend for new revision: $new_rev_backend"
	check_contains "$new_rev_backend" "$initial_backend"

	echo "Checksum deduplication test passed"
}

# test_secrets_hook_commit deploys the charms once and runs all
# CommitHookChanges-based secret scenarios against the same model, to avoid
# the cost of redeploying for each scenario. Each scenario uses its own
# secret URI for isolation.
test_secrets_hook_commit() {
	if [ "$(skip 'test_secrets_hook_commit')" ]; then
		echo "==> TEST SKIPPED: test_secrets_hook_commit"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		model_name='model-secrets-hook-commit'
		add_model "$model_name"

		juju --show-log deploy juju-qa-test
		juju --show-log deploy juju-qa-dummy-source
		juju --show-log deploy juju-qa-dummy-sink
		juju --show-log integrate dummy-sink dummy-source
		wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"
		wait_for "dummy-source" "$(idle_condition "dummy-source" 0)"
		wait_for "dummy-sink" "$(idle_condition "dummy-sink" 0)"

		run_atomic_secret_create
		run_track_latest_revision
		run_atomic_secret_update
		run_atomic_secret_update_grant
		run_checksum_deduplication_no_leak

		destroy_model "$model_name"
	)
}
