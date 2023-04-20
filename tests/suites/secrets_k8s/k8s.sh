run_secrets() {
	echo

	juju --show-log add-model "model-secrets-k8s" --config secret-store=kubernetes

	juju --show-log deploy hello-kubecon hello
	wait_for "active" '.applications["hello"] | ."application-status".current'
	wait_for "hello" "$(idle_condition "hello" 0)"

	secret_owned_by_hello_0=$(juju exec --unit hello/0 -- secret-add --owner unit owned-by=hello/0)
	secret_owned_by_hello_0_id=$(echo $secret_owned_by_hello_0 | awk '{n=split($0,a,"/"); print a[n]}')
	secret_owned_by_hello=$(juju exec --unit hello/0 -- secret-add owned-by=hello-app)
	secret_owned_by_hello_id=$(echo $secret_owned_by_hello | awk '{n=split($0,a,"/"); print a[n]}')

	juju exec --unit hello/0 -- secret-ids | grep "$secret_owned_by_hello"
	juju exec --unit hello/0 -- secret-ids | grep "$secret_owned_by_hello_0"

	echo "Set a label for the unit owned secret $secret_owned_by_hello_0."
	juju exec --unit hello/0 -- secret-set "$secret_owned_by_hello_0" --label=hello_0
	echo "Set a consumer label for the app owned secret $secret_owned_by_hello."
	juju exec --unit hello/0 -- secret-get "$secret_owned_by_hello" --label=hello-app

	# secret-get by URI - content.
	juju exec --unit hello/0 -- secret-get "$secret_owned_by_hello_0" | grep 'owned-by: hello/0'
	juju exec --unit hello/0 -- secret-get "$secret_owned_by_hello" | grep 'owned-by: hello-app'

	# secret-get by URI - metadata.
	juju exec --unit hello/0 -- secret-info-get "$secret_owned_by_hello_0" --format json | jq ".${secret_owned_by_hello_0_id}.owner" | grep unit
	juju exec --unit hello/0 -- secret-info-get "$secret_owned_by_hello" --format json | jq ".${secret_owned_by_hello_id}.owner" | grep application

	# secret-get by label or consumer label - content.
	juju exec --unit hello/0 -- secret-get --label=hello_0 | grep 'owned-by: hello/0'
	juju exec --unit hello/0 -- secret-get --label=hello-app | grep 'owned-by: hello-app'

	# secret-get by label - metadata.
	juju exec --unit hello/0 -- secret-info-get --label=hello_0 --format json | jq ".${secret_owned_by_hello_0_id}.label" | grep hello_0

	juju --show-log deploy nginx-ingress-integrator nginx
	juju --show-log integrate nginx hello
	juju --show-log trust nginx --scope=cluster

	wait_for "active" '.applications["nginx"] | ."application-status".current' 900
	wait_for "nginx" "$(idle_condition "nginx" 1 0)"
	wait_for "active" "$(workload_status "nginx" 0).current"
	wait_for "hello" '.applications["nginx"] | .relations.ingress[0]'

	relation_id=$(juju --show-log show-unit hello/0 --format json | jq '."hello/0"."relation-info"[0]."relation-id"')
	juju exec --unit hello/0 -- secret-grant "$secret_owned_by_hello_0" -r "$relation_id"
	juju exec --unit hello/0 -- secret-grant "$secret_owned_by_hello" -r "$relation_id"

	# secret-get by URI - consume content.
	juju exec --unit nginx/0 -- secret-get "$secret_owned_by_hello_0" | grep 'owned-by: hello/0'
	juju exec --unit nginx/0 -- secret-get "$secret_owned_by_hello" | grep 'owned-by: hello-app'

	juju exec --unit nginx/0 -- secret-get "$secret_owned_by_hello_0" --label=consumer_label_secret_owned_by_hello_0
	juju exec --unit nginx/0 -- secret-get "$secret_owned_by_hello" --label=consumer_label_secret_owned_by_hello

	juju exec --unit nginx/0 -- secret-get --label=consumer_label_secret_owned_by_hello_0 | grep 'owned-by: hello/0'
	juju exec --unit nginx/0 -- secret-get --label=consumer_label_secret_owned_by_hello | grep 'owned-by: hello-app'

	# Check owner unit's k8s role rules to ensure we are using the k8s secret provider.
	microk8s kubectl -n model-secrets-k8s get roles/unit-hello-0 -o json | jq ".rules[] | select( has(\"resourceNames\") ) | select( .resourceNames[] | contains(\"${secret_owned_by_hello_0}-1\") ) | .verbs[0] " | grep -F '*'
	microk8s kubectl -n model-secrets-k8s get roles/unit-hello-0 -o json | jq ".rules[] | select( has(\"resourceNames\") ) | select( .resourceNames[] | contains(\"${secret_owned_by_hello}-1\") ) | .verbs[0] " | grep -F '*'

	# Check consumer unit's k8s role rules to ensure we are using the k8s secret provider.
	microk8s kubectl -n model-secrets-k8s get roles/unit-nginx-0 -o json | jq ".rules[] | select( has(\"resourceNames\") ) | select( .resourceNames[] | contains(\"${secret_owned_by_hello_0}-1\") ) | .verbs[0] " | grep 'get'
	microk8s kubectl -n model-secrets-k8s get roles/unit-nginx-0 -o json | jq ".rules[] | select( has(\"resourceNames\") ) | select( .resourceNames[] | contains(\"${secret_owned_by_hello}-1\") ) | .verbs[0] " | grep 'get'

	microk8s kubectl -n model-secrets-k8s get "secrets/${secret_owned_by_hello_0}-1" -o json | jq -r '.data["owned-by"]' | base64 -d | grep "hello/0"
	microk8s kubectl -n model-secrets-k8s get "secrets/${secret_owned_by_hello}-1" -o json | jq -r '.data["owned-by"]' | base64 -d | grep "hello-app"

	# secret-revoke by relation ID.
	juju exec --unit hello/0 -- secret-revoke "$secret_owned_by_hello" --relation "$relation_id"
	check_contains "$(juju exec --unit nginx/0 -- secret-get "$secret_owned_by_hello" 2>&1)" 'permission denied'

	# secret-revoke by app name.
	juju exec --unit hello/0 -- secret-revoke "$secret_owned_by_hello_0" --app nginx
	check_contains "$(juju exec --unit nginx/0 -- secret-get "$secret_owned_by_hello_0" 2>&1)" 'permission denied'

	# secret-remove.
	juju exec --unit hello/0 -- secret-remove "$secret_owned_by_hello_0"
	check_contains "$(juju exec --unit hello/0 -- secret-get "$secret_owned_by_hello_0" 2>&1)" 'not found'
	juju exec --unit hello/0 -- secret-remove "$secret_owned_by_hello"
	check_contains "$(juju exec --unit hello/0 -- secret-get "$secret_owned_by_hello" 2>&1)" 'not found'

	# TODO: destroy model properly once we fix the k8s teardown issue.
	# We destroy with --force for now.
	juju --show-log destroy-model --no-prompt --debug --force --destroy-storage "model-secrets-k8s"
	# destroy_model "model-secrets-k8s"
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
