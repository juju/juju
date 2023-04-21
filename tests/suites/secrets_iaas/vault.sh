run_secrets_vault() {
	echo

	prepare_vault

	juju add-secret-backend myvault vault endpoint="$VAULT_ADDR" token="$VAULT_TOKEN"
	juju add-model "model-secrets-vault" --config secret-backend=myvault

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

	model_uuid=$(juju models --format json | jq -r '.models[] | select(.name == "admin/model-secrets-vault") | ."model-uuid"')
	vault kv get -format json "${model_uuid}/${secret_owned_by_easyrsa_0}-1" | jq -r '.data."owned-by"' | base64 -d | grep "easyrsa/0"
	vault kv get -format json "${model_uuid}/${secret_owned_by_easyrsa}-1" | jq -r '.data."owned-by"' | base64 -d | grep "easyrsa-app"

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

	destroy_model "model-secrets-vault"
	destroy_model "model-vault-provider"
}

prepare_vault() {
	juju add-model "model-vault-provider"

	if ! which "vault" >/dev/null 2>&1; then
		sudo snap install vault
	fi

	juju --show-log deploy vault
	juju --show-log deploy -n 3 mysql-innodb-cluster
	juju --show-log deploy mysql-router
	juju --show-log integrate mysql-router:db-router mysql-innodb-cluster:db-router
	juju --show-log integrate mysql-router:shared-db vault:shared-db
	juju --show-log expose vault

	wait_for "active" '.applications["mysql-innodb-cluster"] | ."application-status".current' 900
	wait_for "active" '.applications["mysql-router"] | ."application-status".current' 900
	wait_for "blocked" "$(workload_status vault 0).current"
	vault_public_addr=$(juju status --format json | jq -r '.applications.vault.units."vault/0"."public-address"')
	export VAULT_ADDR="http://${vault_public_addr}:8200"
	vault status || true
	vault_init_output=$(vault operator init -key-shares=5 -key-threshold=3 -format json)
	vault_token=$(echo "$vault_init_output" | jq -r .root_token)
	export VAULT_TOKEN="$vault_token"
	unseal_key0=$(echo "$vault_init_output" | jq -r '.unseal_keys_b64[0]')
	unseal_key1=$(echo "$vault_init_output" | jq -r '.unseal_keys_b64[1]')
	unseal_key2=$(echo "$vault_init_output" | jq -r '.unseal_keys_b64[2]')

	vault operator unseal "$unseal_key0"
	vault operator unseal "$unseal_key1"
	vault operator unseal "$unseal_key2"
}

test_secrets_vault() {
	if [ "$(skip 'test_secrets_vault')" ]; then
		echo "==> TEST SKIPPED: test_secrets_vault"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_secrets_vault"
	)
}
