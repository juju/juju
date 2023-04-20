run_secrets_juju() {
	echo

	juju --show-log add-model "model-secrets-juju"

	juju --show-log deploy easyrsa
	juju --show-log deploy etcd
	juju --show-log integrate etcd easyrsa

	wait_for "active" '.applications["easyrsa"] | ."application-status".current'
	wait_for "active" '.applications["etcd"] | ."application-status".current' 900
	wait_for "easyrsa" "$(idle_condition "easyrsa" 0 0)"
	wait_for "etcd" "$(idle_condition "etcd" 1 0)"
	wait_for "active" "$(workload_status "etcd" 0).current"

	secret_owned_by_easyrsa_0=$(juju exec --unit easyrsa/0 -- secret-add --owner unit owned-by=easyrsa/0)
	secret_owned_by_easyrsa_0_id=$(echo $secret_owned_by_easyrsa_0 | awk '{n=split($0,a,"/"); print a[n]}')
	secret_owned_by_easyrsa=$(juju exec --unit easyrsa/0 -- secret-add owned-by=easyrsa-app)
	secret_owned_by_easyrsa_id=$(echo $secret_owned_by_easyrsa | awk '{n=split($0,a,"/"); print a[n]}')

	juju exec --unit easyrsa/0 -- secret-ids | grep $secret_owned_by_easyrsa_id
	juju exec --unit easyrsa/0 -- secret-ids | grep $secret_owned_by_easyrsa_0_id

	echo "Set a label for the unit owned secret $secret_owned_by_easyrsa_0."
	juju exec --unit easyrsa/0 -- secret-set "$secret_owned_by_easyrsa_0" --label=easyrsa_0
	echo "Set a consumer label for the app owned secret $secret_owned_by_easyrsa."
	juju exec --unit easyrsa/0 -- secret-get "$secret_owned_by_easyrsa" --label=easyrsa-app

	# secret-get by URI - content.
	juju exec --unit easyrsa/0 -- secret-get "$secret_owned_by_easyrsa_0" | grep 'owned-by: easyrsa/0'
	juju exec --unit easyrsa/0 -- secret-get "$secret_owned_by_easyrsa" | grep 'owned-by: easyrsa-app'

	# secret-get by URI - metadata.
	juju exec --unit easyrsa/0 -- secret-info-get "$secret_owned_by_easyrsa_0" --format json | jq ".${secret_owned_by_easyrsa_0_id}.owner" | grep unit
	juju exec --unit easyrsa/0 -- secret-info-get "$secret_owned_by_easyrsa" --format json | jq ".${secret_owned_by_easyrsa_id}.owner" | grep application

	# secret-get by label or consumer label - content.
	juju exec --unit easyrsa/0 -- secret-get --label=easyrsa_0 | grep 'owned-by: easyrsa/0'
	juju exec --unit easyrsa/0 -- secret-get --label=easyrsa-app | grep 'owned-by: easyrsa-app'

	# secret-get by label - metadata.
	juju exec --unit easyrsa/0 -- secret-info-get --label=easyrsa_0 --format json | jq ".${secret_owned_by_easyrsa_0_id}.label" | grep easyrsa_0

	relation_id=$(juju --show-log show-unit easyrsa/0 --format json | jq '."easyrsa/0"."relation-info"[0]."relation-id"')
	juju exec --unit easyrsa/0 -- secret-grant "$secret_owned_by_easyrsa_0" -r "$relation_id"
	juju exec --unit easyrsa/0 -- secret-grant "$secret_owned_by_easyrsa" -r "$relation_id"

	# secret-get by URI - consume content.
	juju exec --unit etcd/0 -- secret-get "$secret_owned_by_easyrsa_0" | grep 'owned-by: easyrsa/0'
	juju exec --unit etcd/0 -- secret-get "$secret_owned_by_easyrsa" | grep 'owned-by: easyrsa-app'

	juju exec --unit etcd/0 -- secret-get "$secret_owned_by_easyrsa_0" --label=consumer_label_secret_owned_by_easyrsa_0
	juju exec --unit etcd/0 -- secret-get "$secret_owned_by_easyrsa" --label=consumer_label_secret_owned_by_easyrsa

	juju exec --unit etcd/0 -- secret-get --label=consumer_label_secret_owned_by_easyrsa_0 | grep 'owned-by: easyrsa/0'
	juju exec --unit etcd/0 -- secret-get --label=consumer_label_secret_owned_by_easyrsa | grep 'owned-by: easyrsa-app'

	# secret-revoke by relation ID.
	juju exec --unit easyrsa/0 -- secret-revoke "$secret_owned_by_easyrsa" --relation "$relation_id"
	check_contains "$(juju exec --unit etcd/0 -- secret-get "$secret_owned_by_easyrsa" 2>&1)" 'permission denied'

	# secret-revoke by app name.
	juju exec --unit easyrsa/0 -- secret-revoke "$secret_owned_by_easyrsa_0" --app etcd
	check_contains "$(juju exec --unit etcd/0 -- secret-get "$secret_owned_by_easyrsa_0" 2>&1)" 'permission denied'

	# secret-remove.
	juju exec --unit easyrsa/0 -- secret-remove "$secret_owned_by_easyrsa_0"
	check_contains "$(juju exec --unit easyrsa/0 -- secret-get "$secret_owned_by_easyrsa_0" 2>&1)" 'not found'
	juju exec --unit easyrsa/0 -- secret-remove "$secret_owned_by_easyrsa"
	check_contains "$(juju exec --unit easyrsa/0 -- secret-get "$secret_owned_by_easyrsa" 2>&1)" 'not found'

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
