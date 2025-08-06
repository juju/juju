check_secrets() {
	juju --show-log deploy easyrsa
	juju --show-log deploy etcd
	juju --show-log integrate etcd easyrsa

	wait_for "active" '.applications["easyrsa"] | ."application-status".current'
	wait_for "active" '.applications["etcd"] | ."application-status".current' 900
	wait_for "easyrsa" "$(idle_condition "easyrsa" 0)"
	wait_for "etcd" "$(idle_condition "etcd" 0)"
	wait_for "active" "$(workload_status "etcd" 0).current"

	echo "Apps deployed, creating secrets"
	secret_owned_by_easyrsa_0=$(juju exec --unit easyrsa/0 -- secret-add --owner unit owned-by=easyrsa/0)
	secret_owned_by_easyrsa_0_id=${secret_owned_by_easyrsa_0##*/}
	secret_owned_by_easyrsa=$(juju exec --unit easyrsa/0 -- secret-add owned-by=easyrsa-app)
	secret_owned_by_easyrsa_id=${secret_owned_by_easyrsa##*/}

	echo "Set same content again for $secret_owned_by_easyrsa."
	juju exec --unit easyrsa/0 -- secret-set "$secret_owned_by_easyrsa_id" owned-by=easyrsa-app

	echo "Checking secret ids"
	check_contains "$(juju exec --unit easyrsa/0 -- secret-ids)" "$secret_owned_by_easyrsa_id"
	check_contains "$(juju exec --unit easyrsa/0 -- secret-ids)" "$secret_owned_by_easyrsa_0_id"

	echo "Set a label for the unit owned secret $secret_owned_by_easyrsa_0."
	juju exec --unit easyrsa/0 -- secret-set "$secret_owned_by_easyrsa_0" --label=easyrsa_0
	echo "Set a consumer label for the app owned secret $secret_owned_by_easyrsa."
	juju exec --unit easyrsa/0 -- secret-get "$secret_owned_by_easyrsa" --label=easyrsa-app

	echo "Checking: secret-get by URI - content"
	check_contains "$(juju exec --unit easyrsa/0 -- secret-get "$secret_owned_by_easyrsa_0")" 'owned-by: easyrsa/0'
	check_contains "$(juju exec --unit easyrsa/0 -- secret-get "$secret_owned_by_easyrsa")" 'owned-by: easyrsa-app'

	echo "Checking: secret-get by URI - metadata"
	check_contains "$(juju exec --unit easyrsa/0 -- secret-info-get "$secret_owned_by_easyrsa_0" --format json | jq ".${secret_owned_by_easyrsa_0_id}.owner")" 'unit'
	check_contains "$(juju exec --unit easyrsa/0 -- secret-info-get "$secret_owned_by_easyrsa" --format json | jq ".${secret_owned_by_easyrsa_id}.owner")" 'application'
	check_contains "$(juju exec --unit easyrsa/0 -- secret-info-get "$secret_owned_by_easyrsa" --format json | jq ".${secret_owned_by_easyrsa_id}.revision")" '1'

	echo "Checking: secret-get by label or consumer label - content"
	check_contains "$(juju exec --unit easyrsa/0 -- secret-get --label=easyrsa_0)" 'owned-by: easyrsa/0'
	check_contains "$(juju exec --unit easyrsa/0 -- secret-get --label=easyrsa-app)" 'owned-by: easyrsa-app'

	echo "Checking: secret-get by label - metadata"
	check_contains "$(juju exec --unit easyrsa/0 -- secret-info-get --label=easyrsa_0 --format json | jq ".${secret_owned_by_easyrsa_0_id}.label")" 'easyrsa_0'

	relation_id=$(juju --show-log show-unit easyrsa/0 --format json | jq '."easyrsa/0"."relation-info"[0]."relation-id"')
	juju exec --unit easyrsa/0 -- secret-grant "$secret_owned_by_easyrsa_0" -r "$relation_id"
	juju exec --unit easyrsa/0 -- secret-grant "$secret_owned_by_easyrsa" -r "$relation_id"

	echo "Checking: secret-get by URI - consume content by ID"
	check_contains "$(juju exec --unit etcd/0 -- secret-get "$secret_owned_by_easyrsa_0" --label=consumer_label_secret_owned_by_easyrsa_0)" 'owned-by: easyrsa/0'
	check_contains "$(juju exec --unit etcd/0 -- secret-get "$secret_owned_by_easyrsa" --label=consumer_label_secret_owned_by_easyrsa)" 'owned-by: easyrsa-app'

	echo "Checking: secret-get by URI - consume content by label"
	check_contains "$(juju exec --unit etcd/0 -- secret-get --label=consumer_label_secret_owned_by_easyrsa_0)" 'owned-by: easyrsa/0'
	check_contains "$(juju exec --unit etcd/0 -- secret-get --label=consumer_label_secret_owned_by_easyrsa)" 'owned-by: easyrsa-app'

	echo "Set different content for $secret_owned_by_easyrsa."
	juju exec --unit easyrsa/0 -- secret-set "$secret_owned_by_easyrsa_id" foo=bar
	check_contains "$(juju exec --unit easyrsa/0 -- secret-info-get "$secret_owned_by_easyrsa" --format json | jq ".${secret_owned_by_easyrsa_id}.revision")" '2'
	check_contains "$(juju exec --unit easyrsa/0 -- secret-get --refresh "$secret_owned_by_easyrsa")" 'foo: bar'

	echo "Checking: secret-revoke by relation ID"
	juju exec --unit easyrsa/0 -- secret-revoke "$secret_owned_by_easyrsa" --relation "$relation_id"
	check_contains "$(juju exec --unit etcd/0 -- secret-get "$secret_owned_by_easyrsa" 2>&1)" 'is not allowed to read this secret'

	echo "Checking: secret-revoke by app name"
	juju exec --unit easyrsa/0 -- secret-revoke "$secret_owned_by_easyrsa_0" --app etcd
	check_contains "$(juju exec --unit etcd/0 -- secret-get "$secret_owned_by_easyrsa_0" 2>&1)" 'is not allowed to read this secret'

	echo "Checking: secret-remove"
	juju exec --unit easyrsa/0 -- secret-remove "$secret_owned_by_easyrsa_0"
	check_contains "$(juju exec --unit easyrsa/0 -- secret-get "$secret_owned_by_easyrsa_0" 2>&1)" 'not found'
	juju exec --unit easyrsa/0 -- secret-remove "$secret_owned_by_easyrsa"
	check_contains "$(juju exec --unit easyrsa/0 -- secret-get "$secret_owned_by_easyrsa" 2>&1)" 'not found'
}

run_user_secrets() {
	echo

	model_name=${1}

	app_name='easyrsa-user-secrets'
	juju --show-log deploy easyrsa "$app_name"
	
	wait_for "active" '.applications["easyrsa-user-secrets"] | ."application-status".current'

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
