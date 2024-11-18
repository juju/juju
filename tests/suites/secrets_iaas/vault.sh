# Copyright 2024 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.

run_secrets_vault() {
	echo

	prepare_vault

	juju add-secret-backend myvault vault endpoint="$VAULT_ADDR" token="$VAULT_TOKEN"

	model_name='model-secrets-vault-charm-owned'
	add_model "$model_name"
	juju --show-log model-secret-backend myvault -m "$model_name"

	check_secrets
	destroy_model "$model_name"

	model_name='model-secrets-vault-model-owned'
	add_model "$model_name"
	juju --show-log model-config secret-backend=myvault -m "$model_name"
	run_user_secrets "$model_name"
	destroy_model "$model_name"

	# test remove-secret-backend with force.
	model_name='model-remove-secret-backend-with-force'
	add_model "$model_name"
	juju --show-log model-config secret-backend=myvault -m "$model_name"
	# add a secret to the vault backend to make sure the backend is in-use.
	juju add-secret foo token=1
	check_contains "$(juju show-secret-backend myvault | yq -r .myvault.secrets)" 1
	check_contains "$(juju list-secret-backends --format yaml | yq -r .myvault.secrets)" 1
	check_contains "$(juju remove-secret-backend myvault 2>&1)" 'backend "myvault" still contains secret content'
	juju remove-secret-backend myvault --force
	destroy_model "$model_name"

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

	juju model-secret-backend "$vault_backend_name"

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

	juju model-secret-backend auto

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

run_user_secret_drain() {
	echo

	prepare_vault

	vault_backend_name='myvault'
	juju add-secret-backend "$vault_backend_name" vault endpoint="$VAULT_ADDR" token="$VAULT_TOKEN"

	model_name='model-user-secrets-drain'
	add_model "$model_name"
	juju --show-log model-secret-backend "$vault_backend_name" -m "$model_name"
	model_uuid=$(juju show-model $model_name --format json | jq -r ".[\"${model_name}\"][\"model-uuid\"]")

	juju --show-log deploy easyrsa
	wait_for "active" '.applications["easyrsa"] | ."application-status".current'
	wait_for "easyrsa" "$(idle_condition "easyrsa" 0 0)"

	secret_uri=$(juju --show-log add-secret mysecret owned-by="$model_name-1" --info "this is a user secret")
	secret_short_uri=${secret_uri##*:}

	juju show-secret --reveal "$secret_uri"
	check_contains "$(vault kv list -format json "${model_name}-${model_uuid: -6}" | jq length)" 1

	juju --show-log grant-secret "$secret_uri" easyrsa
	check_contains "$(juju exec --unit easyrsa/0 -- secret-get $secret_short_uri)" "owned-by: $model_name-1"

	# change the secret backend to internal.
	juju model-secret-backend auto

	another_secret_uri=$(juju --show-log add-secret anothersecret owned-by="$model_name-2" --info "this is another user secret")
	juju show-secret --reveal "$another_secret_uri"

	# ensure the user secrets are all in internal backend, no secret in vault.
	attempt=0
	until [[ $(vault kv list -format json "${model_name}-${model_uuid: -6}" | jq length) -eq 0 ]]; do
		if [[ ${attempt} -ge 30 ]]; then
			echo "Failed: expected all secrets get drained back to juju controller."
			exit 1
		fi
		sleep 2
		attempt=$((attempt + 1))
	done

	# change the secret backend to vault.
	juju model-secret-backend "$vault_backend_name"

	# ensure the user secrets are in the vault backend.
	attempt=0
	until [[ $(vault kv list -format json "${model_name}-${model_uuid: -6}" | jq length) -eq 2 ]]; do
		if [[ ${attempt} -ge 30 ]]; then
			echo "Failed: expected all secrets get drained to vault."
			exit 1
		fi
		sleep 2
		attempt=$((attempt + 1))
	done

	# ensure the application can still read the user secret.
	check_contains "$(juju exec --unit easyrsa/0 -- secret-get $secret_short_uri)" "owned-by: $model_name-1"

	juju show-secret --reveal mysecret
	juju show-secret --reveal anothersecret

	destroy_model "$model_name"
	destroy_model "model-vault-provider"
}

prepare_vault() {
	add_model "model-vault-provider"

	if ! which "vault" >/dev/null 2>&1; then
		sudo snap install vault --channel=1.8/stable
	fi

	# If no databases are related, vault will be auto configured to
	# use its embedded raft storage backend for storage and HA.
	juju --show-log deploy vault --channel=1.8/stable
	juju --show-log expose vault

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

	sleep 60

	# wait for vault server to be ready.
	attempt=0
	until [[ $(vault status -format yaml 2>/dev/null | yq .initialized | grep -i 'true') ]]; do
		if [[ ${attempt} -ge 30 ]]; then
			echo "Failed: vault server was not initialized."
			exit 1
		fi
		sleep 2
		attempt=$((attempt + 1))
	done

	attempt=0
	until [[ $(vault status -format yaml 2>/dev/null | yq .ha_enabled | grep -i 'true') ]]; do
		if [[ ${attempt} -ge 30 ]]; then
			echo "Failed: vault server was not HA enabled."
			exit 1
		fi
		sleep 2
		attempt=$((attempt + 1))
	done
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

test_user_secret_drain() {
	if [ "$(skip 'test_user_secret_drain')" ]; then
		echo "==> TEST SKIPPED: test_user_secret_drain"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_user_secret_drain"
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
