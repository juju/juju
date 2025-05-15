run_secrets() {
	echo

	model_name='model-secrets-k8s'
	juju --show-log add-model "$model_name" --config secret-backend=auto

	# k8s secrets are stored in an external backend.
	# These checks ensure the secrets are deleted when the units and app are deleted.
	echo "deploy an app and create an app owned secret and a unit owned secret"
	juju --show-log deploy alertmanager-k8s
	wait_for "alertmanager-k8s" "$(active_idle_condition "alertmanager-k8s" 0 0)"
	wait_for "active" '.applications["alertmanager-k8s"] | ."application-status".current'
	full_uri1=$(juju exec --unit alertmanager-k8s/0 -- secret-add foo=bar)
	short_uri1=${full_uri1##*/}
	full_uri2=$(juju exec --unit alertmanager-k8s/0 -- secret-add --owner unit foo=bar2)
	short_uri2=${full_uri2##*/}
	check_contains "$(microk8s kubectl -n "$model_name" get secrets -o json | jq -r '.items[].metadata.name | select(. == "'"${short_uri1}"'-1")')" "${short_uri1}-1"
	check_contains "$(microk8s kubectl -n "$model_name" get secrets -o json | jq -r '.items[].metadata.name | select(. == "'"${short_uri2}"'-1")')" "${short_uri2}-1"

	echo "add another unit and create a unit owned secret"
	juju --show-log scale-application alertmanager-k8s 2
	wait_for "alertmanager-k8s" "$(active_idle_condition "alertmanager-k8s" 0 1)"
	full_uri3=$(juju exec --unit alertmanager-k8s/1 -- secret-add --owner unit foo=bar3)
	short_uri3=${full_uri3##*/}
	check_contains "$(microk8s kubectl -n "$model_name" get secrets -o json | jq -r '.items[].metadata.name | select(. == "'"${short_uri3}"'-1")')" "${short_uri3}-1"

	echo "remove a unit and check only its secret is removed"
	juju --show-log scale-application alertmanager-k8s 1
	check_contains "$(microk8s kubectl -n "$model_name" get secrets -o json | jq -r '.items[].metadata.name | select(. == "'"${short_uri1}"'-1")')" "${short_uri1}-1"
	check_contains "$(microk8s kubectl -n "$model_name" get secrets -o json | jq -r '.items[].metadata.name | select(. == "'"${short_uri2}"'-1")')" "${short_uri2}-1"
	attempt=0
	until [[ -z $(microk8s kubectl -n "$model_name" get secrets -o json | jq -r '.items[].metadata.name | select(. == "'"${short_uri3}"'-1")') ]]; do
		if [[ ${attempt} -ge 30 ]]; then
			echo "Failed: secrets were not deleted on unit removal."
			exit 1
		fi
		sleep 2
		attempt=$((attempt + 1))
	done

	echo "remove the last unit and check only the app owned secret remains"
	juju --show-log scale-application alertmanager-k8s 0
	check_contains "$(microk8s kubectl -n "$model_name" get secrets -o json | jq -r '.items[].metadata.name | select(. == "'"${short_uri1}"'-1")')" "${short_uri1}-1"
	attempt=0
	until [[ -z $(microk8s kubectl -n "$model_name" get secrets -o json | jq -r '.items[].metadata.name | select(. == "'"${short_uri2}"'-1")') ]]; do
		if [[ ${attempt} -ge 30 ]]; then
			echo "Failed: secrets were not deleted on unit removal."
			exit 1
		fi
		sleep 2
		attempt=$((attempt + 1))
	done

	echo "remove the app and the app owned secret should be deleted too"
	juju --show-log remove-application alertmanager-k8s
	attempt=0
	until [[ -z $(microk8s kubectl -n "$model_name" get secrets -o json | jq -r '.items[].metadata.name | select(. == "'"${short_uri1}"'-1")') ]]; do
		if [[ ${attempt} -ge 30 ]]; then
			echo "Failed: secrets were not deleted on app removal."
			exit 1
		fi
		sleep 2
		attempt=$((attempt + 1))
	done

	juju --show-log deploy alertmanager-k8s hello
	# TODO(anvial): remove the revision flag once we update alertmanager-k8s charm
	#  (https://discourse.charmhub.io/t/old-ingress-relation-removal/12944)
	#  or we choose an alternative pair of charms to integrate.
	juju --show-log deploy nginx-ingress-integrator nginx --channel=latest/stable --revision=83
	juju --show-log integrate nginx hello
	juju --show-log trust nginx --scope=cluster

	# create user secrets.
	juju --show-log add-secret mysecret owned-by="$model_name" --info "this is a user secret"

	wait_for "active" '.applications["hello"] | ."application-status".current'
	wait_for "hello" "$(idle_condition "hello" 0)"
	wait_for "active" '.applications["nginx"] | ."application-status".current' 900
	wait_for "nginx" "$(idle_condition "nginx" 1 0)"
	wait_for "active" "$(workload_status "nginx" 0).current"
	wait_for "hello" '.applications["nginx"] | .relations.ingress[0]'

	echo "Apps deployed, creating secrets"
	unit_owned_full_uri=$(juju exec --unit hello/0 -- secret-add --owner unit owned-by=hello/0)
	unit_owned_short_uri=${unit_owned_full_uri##*/}
	app_owned_full_uri=$(juju exec --unit hello/0 -- secret-add owned-by=hello-app)
	app_owned_short_uri=${app_owned_full_uri##*/}

	echo "Checking: secret ids"
	check_contains "$(juju exec --unit hello/0 -- secret-ids)" "$app_owned_short_uri"
	check_contains "$(juju exec --unit hello/0 -- secret-ids)" "$unit_owned_short_uri"

	echo "Set a label for the unit owned secret $unit_owned_full_uri."
	juju exec --unit hello/0 -- secret-set "$unit_owned_full_uri" --label=hello_0
	echo "Set a consumer label for the app owned secret $app_owned_full_uri."
	juju exec --unit hello/0 -- secret-get "$app_owned_full_uri" --label=hello-app

	echo "Checking: secret-get by URI - content"
	check_contains "$(juju exec --unit hello/0 -- secret-get $unit_owned_full_uri)" 'owned-by: hello/0'
	check_contains "$(juju exec --unit hello/0 -- secret-get $app_owned_full_uri)" 'owned-by: hello-app'

	echo "Checking: secret-get by URI - metadata"
	check_contains "$(juju exec --unit hello/0 -- secret-info-get $unit_owned_full_uri --format json | jq .${unit_owned_short_uri}.owner)" unit
	check_contains "$(juju exec --unit hello/0 -- secret-info-get $app_owned_full_uri --format json | jq .${app_owned_short_uri}.owner)" application

	echo "Checking: secret-get by label or consumer label - content"
	check_contains "$(juju exec --unit hello/0 -- secret-get --label=hello_0)" 'owned-by: hello/0'
	check_contains "$(juju exec --unit hello/0 -- secret-get --label=hello-app)" 'owned-by: hello-app'

	echo "Checking: secret-get by label - metadata"
	check_contains "$(juju exec --unit hello/0 -- secret-info-get --label=hello_0 --format json | jq ".${unit_owned_short_uri}.label")" hello_0

	relation_id=$(juju --show-log show-unit hello/0 --format json | jq '."hello/0"."relation-info"[0]."relation-id"')
	juju exec --unit hello/0 -- secret-grant "$unit_owned_full_uri" -r "$relation_id"
	juju exec --unit hello/0 -- secret-grant "$app_owned_full_uri" -r "$relation_id"

	echo "Checking: secret-get by URI - consume content"
	check_contains "$(juju exec --unit nginx/0 -- secret-get "$unit_owned_full_uri")" 'owned-by: hello/0'
	check_contains "$(juju exec --unit nginx/0 -- secret-get "$app_owned_full_uri")" 'owned-by: hello-app'

	juju exec --unit nginx/0 -- secret-get "$unit_owned_full_uri" --label=consumer_label_secret_owned_by_hello_0
	juju exec --unit nginx/0 -- secret-get "$app_owned_full_uri" --label=consumer_label_secret_owned_by_hello

	check_contains "$(juju exec --unit nginx/0 -- secret-get --label=consumer_label_secret_owned_by_hello_0)" 'owned-by: hello/0'
	check_contains "$(juju exec --unit nginx/0 -- secret-get --label=consumer_label_secret_owned_by_hello)" 'owned-by: hello-app'

	echo "Check owner unit's k8s role rules to ensure we are using the k8s secret provider"
	check_contains "$(microk8s kubectl -n "$model_name" get roles/unit-hello-0 -o json | jq ".rules[] | select( has(\"resourceNames\") ) | select( .resourceNames[] | contains(\"${unit_owned_short_uri}-1\") ) | .verbs[0] ")" '*'
	check_contains "$(microk8s kubectl -n "$model_name" get roles/unit-hello-0 -o json | jq ".rules[] | select( has(\"resourceNames\") ) | select( .resourceNames[] | contains(\"${app_owned_short_uri}-1\") ) | .verbs[0] ")" '*'

	# Check consumer unit's k8s role rules to ensure we are using the k8s secret provider.
	check_contains "$(microk8s kubectl -n "$model_name" get roles/unit-nginx-0 -o json | jq ".rules[] | select( has(\"resourceNames\") ) | select( .resourceNames[] | contains(\"${unit_owned_short_uri}-1\") ) | .verbs[0] ")" 'get'
	check_contains "$(microk8s kubectl -n "$model_name" get roles/unit-nginx-0 -o json | jq ".rules[] | select( has(\"resourceNames\") ) | select( .resourceNames[] | contains(\"${app_owned_short_uri}-1\") ) | .verbs[0] ")" 'get'

	check_contains "$(microk8s kubectl -n "$model_name" get "secrets/${unit_owned_short_uri}-1" -o json | jq -r '.data["owned-by"]' | base64 -d)" "hello/0"
	check_contains "$(microk8s kubectl -n "$model_name" get "secrets/${app_owned_short_uri}-1" -o json | jq -r '.data["owned-by"]' | base64 -d)" "hello-app"

	echo "Checking: secret-revoke by relation ID"
	juju exec --unit hello/0 -- secret-revoke "$app_owned_full_uri" --relation "$relation_id"
	check_contains "$(juju exec --unit nginx/0 -- secret-get "$app_owned_full_uri" 2>&1)" 'permission denied'

	echo "Checking: secret-revoke by app name"
	juju exec --unit hello/0 -- secret-revoke "$unit_owned_short_uri" --app nginx
	check_contains "$(juju exec --unit nginx/0 -- secret-get "$unit_owned_short_uri" 2>&1)" 'permission denied'

	echo "Checking: secret-remove"
	juju exec --unit hello/0 -- secret-remove "$unit_owned_short_uri"
	check_contains "$(juju exec --unit hello/0 -- secret-get "$unit_owned_short_uri" 2>&1)" 'not found'
	juju exec --unit hello/0 -- secret-remove "$app_owned_full_uri"
	check_contains "$(juju exec --unit hello/0 -- secret-get "$app_owned_full_uri" 2>&1)" 'not found'

	# TODO: no need to remove-relation before destroying model once we fixed(lp:1952221).
	juju --show-log remove-relation nginx hello
	# wait for relation removed.
	wait_for null '.applications["nginx"] | .relations.source[0]'
	destroy_model "$model_name"
}

run_user_secrets() {
	echo

	model_name='model-user-secrets-k8s'
	juju --show-log add-model "$model_name" --config secret-backend=auto
	model_uuid=$(juju show-model $model_name --format json | jq -r ".[\"${model_name}\"][\"model-uuid\"]")

	juju --show-log deploy alertmanager-k8s

	# create user secrets.
	secret_uri=$(juju --show-log add-secret mysecret owned-by="$model_name-1" --info "this is a user secret")
	secret_short_uri=${secret_uri##*:}

	check_contains "$(juju --show-log show-secret "$secret_uri" --revisions | yq ".${secret_short_uri}.description")" 'this is a user secret'

	# create a new revision 2.
	juju --show-log update-secret "$secret_uri" --info info owned-by="$model_name-2"
	check_contains "$(juju --show-log show-secret "$secret_uri" --revisions | yq ".${secret_short_uri}.description")" 'info'

	# grant secret to alertmanager-k8s app, and now the application can access the revision 2.
	check_contains "$(juju exec --unit alertmanager-k8s/0 -- secret-get "$secret_uri" 2>&1)" 'permission denied'
	juju --show-log grant-secret "$secret_uri" alertmanager-k8s
	check_contains "$(juju exec --unit alertmanager-k8s/0 -- secret-get $secret_short_uri)" "owned-by: $model_name-2"

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
	juju --show-log update-secret "$secret_uri" --auto-prune=true

	# revision 1 should be pruned.
	# revision 2 is still been used by alertmanager-k8s app, so it should not be pruned.
	# revision 3 is the latest revision, so it should not be pruned.
	check_num_secret_revisions "$secret_uri" "$secret_short_uri" 2
	check_contains "$(juju --show-log show-secret $secret_uri --reveal --revision 2 | yq .${secret_short_uri}.content)" "owned-by: $model_name-2"
	check_contains "$(juju --show-log show-secret $secret_uri --reveal --revision 3 | yq .${secret_short_uri}.content)" "owned-by: $model_name-3"

	check_contains "$(juju exec --unit alertmanager-k8s/0 -- secret-get $secret_short_uri --peek)" "owned-by: $model_name-3"
	check_contains "$(juju exec --unit alertmanager-k8s/0 -- secret-get $secret_short_uri --refresh)" "owned-by: $model_name-3"
	check_contains "$(juju exec --unit alertmanager-k8s/0 -- secret-get $secret_short_uri)" "owned-by: $model_name-3"

	# revision 2 should be pruned.
	# revision 3 is the latest revision, so it should not be pruned.
	check_num_secret_revisions "$secret_uri" "$secret_short_uri" 1
	check_contains "$(juju --show-log show-secret $secret_uri --reveal --revision 3 | yq .${secret_short_uri}.content)" "owned-by: $model_name-3"

	juju --show-log revoke-secret $secret_uri alertmanager-k8s
	check_contains "$(juju exec --unit alertmanager-k8s/0 -- secret-get "$secret_uri" 2>&1)" 'permission denied'

	juju --show-log remove-secret $secret_uri
	check_contains "$(juju --show-log secrets --format yaml | yq length)" '0'
}

run_secret_drain() {
	model_name='model-secrets-k8s-drain'
	juju --show-log add-model "$model_name"

	prepare_vault
	vault_backend_name='myvault'
	juju add-secret-backend "$vault_backend_name" vault endpoint="$VAULT_ADDR" token="$VAULT_TOKEN"

	juju --show-log deploy alertmanager-k8s hello
	wait_for "active" '.applications["hello"] | ."application-status".current'
	wait_for "hello" "$(idle_condition "hello" 0)"

	unit_owned_full_uri=$(juju exec --unit hello/0 -- secret-add --owner unit owned-by=hello/0)
	unit_owned_short_uri=${unit_owned_full_uri##*/}
	app_owned_full_uri=$(juju exec --unit hello/0 -- secret-add owned-by=hello-app)
	app_owned_short_uri=${app_owned_full_uri##*/}

	juju show-secret --reveal "$unit_owned_full_uri"
	juju show-secret --reveal "$app_owned_full_uri"

	check_contains "$(microk8s kubectl -n "$model_name" get secrets -l 'app.juju.is/created-by=hello')" "${unit_owned_short_uri}-1"
	check_contains "$(microk8s kubectl -n "$model_name" get secrets -l 'app.juju.is/created-by=hello')" "${app_owned_short_uri}-1"

	juju model-config secret-backend="$vault_backend_name"

	attempt=0
	until [[ $(microk8s kubectl -n "$model_name" get secrets -l 'app.juju.is/created-by=hello' -o json | jq '.items | length') -eq 0 ]]; do
		if [[ ${attempt} -ge 30 ]]; then
			echo "Failed: expected all secrets get drained to vault, so k8s has no secrets."
			exit 1
		fi
		sleep 2
		attempt=$((attempt + 1))
	done

	model_uuid=$(juju show-model $model_name --format json | jq -r ".[\"${model_name}\"][\"model-uuid\"]")
	check_contains "$(vault kv list -format json "${model_name}-${model_uuid: -6}" | jq length)" 2

	juju model-config secret-backend=auto

	attempt=0
	until [[ "$(microk8s kubectl -n $model_name get secrets -l 'app.juju.is/created-by=hello')" =~ ${unit_owned_short_uri}-1 ]]; do
		if [[ ${attempt} -ge 30 ]]; then
			echo "Failed: expected secret ${unit_owned_short_uri}-1 gets drained to k8s."
			exit 1
		fi
		sleep 2
		attempt=$((attempt + 1))
	done

	attempt=0
	until [[ "$(microk8s kubectl -n $model_name get secrets -l 'app.juju.is/created-by=hello')" =~ ${app_owned_short_uri}-1 ]]; do
		if [[ ${attempt} -ge 30 ]]; then
			echo "Failed: expected secret ${app_owned_short_uri}-1 gets drained to k8s."
			exit 1
		fi
		sleep 2
		attempt=$((attempt + 1))
	done

	check_contains "$(vault kv list -format json "${model_name}-${model_uuid: -6}" | jq length)" 0

	destroy_model "$model_name"
}

run_user_secret_drain() {
	model_name='model-user-secrets-k8s-drain'
	juju --show-log add-model "$model_name"
	model_uuid=$(juju show-model $model_name --format json | jq -r ".[\"${model_name}\"][\"model-uuid\"]")

	prepare_vault
	vault_backend_name='myvault'
	juju add-secret-backend "$vault_backend_name" vault endpoint="$VAULT_ADDR" token="$VAULT_TOKEN"

	juju --show-log deploy alertmanager-k8s hello
	wait_for "active" '.applications["hello"] | ."application-status".current'
	wait_for "hello" "$(idle_condition "hello" 0)"

	secret_uri=$(juju --show-log add-secret mysecret owned-by="$model_name-1" --info "this is a user secret")
	secret_short_uri=${secret_uri##*:}

	juju show-secret --reveal "$secret_uri"

	juju --show-log grant-secret "$secret_uri" hello
	check_contains "$(juju exec --unit hello/0 -- secret-get $secret_short_uri)" "owned-by: $model_name-1"

	check_contains "$(microk8s kubectl -n "$model_name" get secrets -l "app.juju.is/created-by=$model_uuid" -o jsonpath='{.items[*].metadata.name}')" "${secret_short_uri}-1"

	juju model-config secret-backend="$vault_backend_name"

	# ensure the user secret is removed from k8s backend.
	attempt=0
	until [[ $(microk8s kubectl -n "$model_name" get secrets -l "app.juju.is/created-by=$model_uuid" -o json | jq '.items | length') -eq 0 ]]; do
		if [[ ${attempt} -ge 30 ]]; then
			echo "Failed: expected all secrets get drained to vault, so k8s has no secrets."
			exit 1
		fi
		sleep 2
		attempt=$((attempt + 1))
	done

	model_uuid=$(juju show-model $model_name --format json | jq -r ".[\"${model_name}\"][\"model-uuid\"]")
	# ensure the user secret is in vault backend.
	check_contains "$(vault kv list -format json "${model_name}-${model_uuid: -6}" | jq length)" 1
	# ensure the application can still read the user secret.
	check_contains "$(juju exec --unit hello/0 -- secret-get $secret_short_uri)" "owned-by: $model_name-1"

	juju model-config secret-backend=auto

	# ensure the user secret is drained back to k8s backend.
	attempt=0
	until [[ "$(microk8s kubectl -n $model_name get secrets -l "app.juju.is/created-by=$model_uuid")" =~ ${secret_short_uri}-1 ]]; do
		if [[ ${attempt} -ge 30 ]]; then
			echo "Failed: expected secret ${secret_short_uri}-1 gets drained to k8s."
			exit 1
		fi
		sleep 2
		attempt=$((attempt + 1))
	done

	# ensure the user secret is removed from vault backend.
	check_contains "$(vault kv list -format json "${model_name}-${model_uuid: -6}" | jq length)" 0
	# ensure the application can still read the user secret.
	check_contains "$(juju exec --unit hello/0 -- secret-get $secret_short_uri)" "owned-by: $model_name-1"

	destroy_model "$model_name"
}

prepare_vault() {
	if ! which "vault" >/dev/null 2>&1; then
		sudo snap install vault
	fi

	ip=$(hostname -I | awk '{print $1}')
	root_token='root'
	timeout 45m vault server -dev -dev-listen-address="${ip}:8200" -dev-root-token-id="$root_token" &

	export VAULT_ADDR="http://${ip}:8200"
	export VAULT_TOKEN="$root_token"

	# wait for vault server to be ready.
	attempt=0
	until [[ $(vault status -format yaml 2>/dev/null | yq .initialized | grep -i 'true') ]]; do
		if [[ ${attempt} -ge 30 ]]; then
			echo "Failed: vault server was not able to be ready."
			exit 1
		fi
		sleep 2
		attempt=$((attempt + 1))
	done
}

test_secrets() {
	if [ "$(skip 'test_secrets')" ]; then
		echo "==> TEST SKIPPED: test_secrets"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_secrets"
	)
}

test_user_secrets() {
	if [ "$(skip 'test_user_secrets')" ]; then
		echo "==> TEST SKIPPED: test_user_secrets"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_user_secrets"
	)
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
