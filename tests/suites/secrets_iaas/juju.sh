check_secrets() {
	juju --show-log deploy easyrsa
	juju --show-log deploy etcd
	juju --show-log integrate etcd easyrsa

	wait_for "active" '.applications["easyrsa"] | ."application-status".current'
	wait_for "active" '.applications["etcd"] | ."application-status".current' 900
	wait_for "easyrsa" "$(idle_condition "easyrsa" 0 0)"
	wait_for "etcd" "$(idle_condition "etcd" 1 0)"
	wait_for "active" "$(workload_status "etcd" 0).current"

	echo "Apps deployed, creating secrets"
	secret_owned_by_easyrsa_0=$(juju exec --unit easyrsa/0 -- secret-add --owner unit owned-by=easyrsa/0)
	secret_owned_by_easyrsa_0_id=${secret_owned_by_easyrsa_0##*/}
	secret_owned_by_easyrsa=$(juju exec --unit easyrsa/0 -- secret-add owned-by=easyrsa-app)
	secret_owned_by_easyrsa_id=${secret_owned_by_easyrsa##*/}

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

	echo "Checking: secret-revoke by relation ID"
	juju exec --unit easyrsa/0 -- secret-revoke "$secret_owned_by_easyrsa" --relation "$relation_id"
	check_contains "$(juju exec --unit etcd/0 -- secret-get "$secret_owned_by_easyrsa" 2>&1)" 'permission denied'

	echo "Checking: secret-revoke by app name"
	juju exec --unit easyrsa/0 -- secret-revoke "$secret_owned_by_easyrsa_0" --app etcd
	check_contains "$(juju exec --unit etcd/0 -- secret-get "$secret_owned_by_easyrsa_0" 2>&1)" 'permission denied'

	echo "Checking: secret-remove"
	juju exec --unit easyrsa/0 -- secret-remove "$secret_owned_by_easyrsa_0"
	check_contains "$(juju exec --unit easyrsa/0 -- secret-get "$secret_owned_by_easyrsa_0" 2>&1)" 'not found'
	juju exec --unit easyrsa/0 -- secret-remove "$secret_owned_by_easyrsa"
	check_contains "$(juju exec --unit easyrsa/0 -- secret-get "$secret_owned_by_easyrsa" 2>&1)" 'not found'
}

run_secrets_juju() {
	echo

	add_model "model-secrets-juju"
	check_secrets
	destroy_model "model-secrets-juju"
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
