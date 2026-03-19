# Copyright 2025 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.

run_secrets_k8s() {
	echo

	prepare_k8s

	juju add-secret-backend myk8s kubernetes --config "${TEST_DIR}/k8sconfig.yaml"

	model_name='model-secrets-k8s-charm-owned'
	add_model "$model_name"
	juju --show-log model-config secret-backend=myk8s -m "$model_name"
	check_secrets
	destroy_model "$model_name"

	model_name='model-secrets-k8s-model-owned'
	add_model "$model_name"
	juju --show-log model-config secret-backend=myk8s -m "$model_name"
	run_user_secrets "$model_name"
	destroy_model "$model_name"

	# test remove-secret-backend with force.
	model_name='model-remove-secret-backend-with-force'
	add_model "$model_name"
	juju --show-log model-config secret-backend=myk8s -m "$model_name"
	# add a secret to the k8s backend to make sure the backend is in-use.
	juju add-secret foo token=1
	check_contains "$(juju show-secret-backend myk8s | yq -r .myk8s.secrets)" 1
	check_contains "$(juju list-secret-backends --format yaml | yq -r .myk8s.secrets)" 1
	check_contains "$(juju remove-secret-backend myk8s 2>&1)" 'backend "myk8s" still contains secret content'
	juju remove-secret-backend myk8s --force
	destroy_model "$model_name"
}

prepare_k8s() {
	if ! kubectl wait --for=jsonpath='{.status.phase}'=Active ns/kube-system; then
		if ! which "microk8s" >/dev/null 2>&1; then
			sudo snap install microk8s --channel 1.32-strict
			sudo microk8s.enable hostpath-storage
			sudo microk8s.enable rbac
			sudo microk8s status --wait-ready
		fi
	fi
	if ! kubectl wait --for=jsonpath='{.status.phase}'=Active ns/kube-system; then
		echo "No suitable kubernetes cluster for test"
		exit 1
	fi

	endpoint=$(kubectl config view --raw --flatten | yq ".clusters[0] .cluster .server")
	cacert=$(kubectl config view --raw --flatten | yq ".clusters[0] .cluster .certificate-authority-data" | base64 -d | sed 's/^/  /')
	namespace=juju-secrets
	serviceaccount=default
	kubectl create ns ${namespace} --dry-run=client -o yaml | kubectl apply -f -
	kubectl create --save-config -n ${namespace} serviceaccount ${serviceaccount} --dry-run=client -o yaml | kubectl apply -f -
	kubectl create --save-config clusterrole juju-secrets --verb='*' \
		--resource=namespaces,secrets,serviceaccounts,serviceaccounts/token,clusterroles,clusterrolebindings --dry-run=client -o yaml | kubectl apply -f -
	kubectl create --save-config clusterrolebinding juju-secrets --clusterrole=juju-secrets \
		--serviceaccount=${namespace}:${serviceaccount} --dry-run=client -o yaml | kubectl apply -f -
	kubectl create --save-config role juju-secrets --namespace=${namespace} --verb='*' \
		--resource=secrets,serviceaccounts,serviceaccounts/token,roles,rolebindings --dry-run=client -o yaml | kubectl apply -f -
	kubectl create --save-config rolebinding juju-secrets --namespace=${namespace} --role=juju-secrets \
		--serviceaccount=${namespace}:${serviceaccount} --dry-run=client -o yaml | kubectl apply -f -
	token=$(kubectl create token ${serviceaccount} --namespace ${namespace})

	cat >"${TEST_DIR}/k8sconfig.yaml" <<EOF
endpoint: ${endpoint}
namespace: ${namespace}
ca-certs:
- |
${cacert}
token: ${token}

EOF
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
