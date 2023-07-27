run_secrets_vault() {
	echo

	prepare_vault

	model_name='model-secrets-vault'
	juju add-secret-backend myvault vault endpoint="$VAULT_ADDR" token="$VAULT_TOKEN"
	add_model "$model_name" --config secret-backend=myvault

	check_secrets

	destroy_model "model-secrets-vault"
	destroy_model "model-vault-provider"
}

run_secret_drain() {
	echo

	prepare_vault

	model_name='model-secrets-drain'
	add_model "$model_name"

	vault_backend_name='myvault'
	juju add-secret-backend "$vault_backend_name" vault endpoint="$VAULT_ADDR" token="$VAULT_TOKEN"

	juju --show-log deploy easyrsa
	wait_for "active" '.applications["easyrsa"] | ."application-status".current'
	wait_for "easyrsa" "$(idle_condition "easyrsa" 0 0)"

	secret_owned_by_unit=$(juju exec --unit easyrsa/0 -- secret-add --owner unit owned-by=easyrsa/0)
	secret_owned_by_app=$(juju exec --unit easyrsa/0 -- secret-add owned-by=easyrsa-app)

	juju show-secret --reveal "$secret_owned_by_unit"
	juju show-secret --reveal "$secret_owned_by_app"

	juju model-config secret-backend="$vault_backend_name"

	model_uuid=$(juju show-model $model_name --format json | jq -r ".[\"${model_name}\"][\"model-uuid\"]")

	attempt=0
	until [[ $(vault kv list -format json "${model_name}-${model_uuid: -6}" | jq length) -eq 2 ]]; do
		if [[ ${attempt} -ge 30 ]]; then
			echo "Failed: expected all secrets get drained to vault."
			exit 1
		fi
		sleep 2
		attempt=$((attempt + 1))
	done

	juju model-config secret-backend=auto

	attempt=0
	until [[ $(vault kv list -format json "${model_name}-${model_uuid: -6}" | jq length) -eq 0 ]]; do
		if [[ ${attempt} -ge 30 ]]; then
			echo "Failed: expected all secrets get drained back to juju controller."
			exit 1
		fi
		sleep 2
		attempt=$((attempt + 1))
	done

	juju show-secret --reveal "$secret_owned_by_unit"
	juju show-secret --reveal "$secret_owned_by_app"

	destroy_model "$model_name"
	destroy_model "model-vault-provider"
}

prepare_vault() {
	add_model "model-vault-provider"

	if ! which "vault" >/dev/null 2>&1; then
		sudo snap install vault
	fi

	juju --show-log deploy vault
	juju --show-log deploy -n 3 mysql-innodb-cluster
	juju --show-log deploy mysql-router
	juju --show-log integrate mysql-router:db-router mysql-innodb-cluster:db-router
	juju --show-log integrate mysql-router:shared-db vault:shared-db
	juju --show-log expose vault

	wait_for "active" '.applications["mysql-innodb-cluster"] | ."application-status".current' 1200
	wait_for "active" '.applications["mysql-router"] | ."application-status".current' 1200
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

test_secret_drain() {
	if [ "$(skip 'test_secret_drain')" ]; then
		echo "==> TEST SKIPPED: test_secret_drain"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_secret_drain"
	)
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
