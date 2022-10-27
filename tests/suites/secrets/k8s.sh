run_secrets_k8s() {
	echo

	file="${TEST_DIR}/model-secrets-k8s.txt"
	juju --show-log add-model "model-secrets-k8s" --config secret-store=kubernetes

	juju --show-log deploy hello-kubecon hello
	wait_for "active" '.applications["hello"] | ."application-status".current'
	wait_for "hello" "$(idle_condition "hello" 0)"

	secret_owned_by_hello_0=$(juju exec --unit hello/0 -- secret-add --owner unit owned-by=hello/0 | cut -d: -f2)
	secret_owned_by_hello=$(juju exec --unit hello/0 -- secret-add owned-by=hello-app | cut -d: -f2)

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
	juju exec --unit hello/0 -- secret-get "$secret_owned_by_hello_0" --metadata --format json | jq ".${secret_owned_by_hello_0}.owner" | grep unit
	juju exec --unit hello/0 -- secret-get "$secret_owned_by_hello" --metadata --format json | jq ".${secret_owned_by_hello}.owner" | grep application

	# secret-get by label or consumer label - content.
	juju exec --unit hello/0 -- secret-get --label=hello_0 | grep 'owned-by: hello/0'
	juju exec --unit hello/0 -- secret-get --label=hello-app | grep 'owned-by: hello-app'

	# secret-get by label - metadata.
	juju exec --unit hello/0 -- secret-get --label=hello_0 --metadata --format json | jq ".${secret_owned_by_hello_0}.label" | grep hello_0

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

	# Check owner unit's k8s role rules.
	microk8s kubectl -n model-secrets-k8s get roles/unit-hello-0 -o json | jq ".rules[] | select( has(\"resourceNames\") ) | select( .resourceNames[] | contains(\"${secret_owned_by_hello_0}-1\") ) | .verbs[0] " | grep '*'
	microk8s kubectl -n model-secrets-k8s get roles/unit-hello-0 -o json | jq ".rules[] | select( has(\"resourceNames\") ) | select( .resourceNames[] | contains(\"${secret_owned_by_hello}-1\") ) | .verbs[0] " | grep '*'

	# Check consumer unit's k8s role rules.
	microk8s kubectl -n model-secrets-k8s get roles/unit-nginx-0 -o json | jq ".rules[] | select( has(\"resourceNames\") ) | select( .resourceNames[] | contains(\"${secret_owned_by_hello_0}-1\") ) | .verbs[0] " | grep 'get'
	microk8s kubectl -n model-secrets-k8s get roles/unit-nginx-0 -o json | jq ".rules[] | select( has(\"resourceNames\") ) | select( .resourceNames[] | contains(\"${secret_owned_by_hello}-1\") ) | .verbs[0] " | grep 'get'

	# Is the data base64 encoded twice??!!
	microk8s kubectl -n model-secrets-k8s get "secrets/${secret_owned_by_hello_0}-1" -o json | jq -r '.data["owned-by"]' | base64 -d | base64 -d | grep "hello/0"
	microk8s kubectl -n model-secrets-k8s get "secrets/${secret_owned_by_hello}-1" -o json | jq -r '.data["owned-by"]' | base64 -d | base64 -d | grep "hello-app"

	# secret-revoke by relation ID.
	juju exec --unit hello/0 -- secret-revoke $secret_owned_by_hello --relation "$relation_id"
	echo $(juju exec --unit nginx/0 -- secret-get "$secret_owned_by_hello" 2>&1) | grep 'permission denied'

	# secret-revoke by app name.
	juju exec --unit hello/0 -- secret-revoke "$secret_owned_by_hello_0" --app nginx
	echo $(juju exec --unit nginx/0 -- secret-get "$secret_owned_by_hello_0" 2>&1) | grep 'permission denied'

	# secret-remove.
	# TODO: enable once we fix: https://bugs.launchpad.net/juju/+bug/1994971.
	# juju exec --unit hello/0 -- secret-remove "$secret_owned_by_hello_0"
	# echo $(juju exec --unit hello/0 -- secret-get "$secret_owned_by_hello_0" 2>&1) | grep 'not found'

	# TODO: enable once we fix: https://bugs.launchpad.net/juju/+bug/1994919.
	# juju exec --unit hello/0 -- secret-remove "$secret_owned_by_hello"
	# juju exec --unit hello/0 -- secret-get "$secret_owned_by_hello" | grep 'not found'

	destroy_model "model-secrets-k8s"
}

test_secrets_k8s() {
	if [ "$(skip 'test_secrets_k8s')" ]; then
		echo "==> TEST SKIPPED: test_secrets_k8s"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_secrets_k8s"
	)
}
