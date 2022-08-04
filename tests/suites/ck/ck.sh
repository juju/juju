run_deploy_ck() {
	echo

	local name model_name file overlay_path kube_home storage_path
	name="${2}"
	model_name="${name}"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	overlay_path="./tests/suites/ck/overlay/${BOOTSTRAP_PROVIDER}.yaml"
	juju deploy charmed-kubernetes --overlay "${overlay_path}" --trust

	if ! which "kubectl" >/dev/null 2>&1; then
		sudo snap install kubectl --classic --channel latest/stable
	fi

	wait_for "active" '.applications["kubernetes-control-plane"] | ."application-status".current'
	wait_for "active" '.applications["kubernetes-worker"] | ."application-status".current'

	kube_home="${HOME}/.kube"
	mkdir -p "${kube_home}"
	juju scp kubernetes-control-plane/0:config "${kube_home}/config"

	kubectl cluster-info
	kubectl get ns
	storage_path="./tests/suites/ck/storage/${BOOTSTRAP_PROVIDER}.yaml"
	kubectl create -f "${storage_path}"
	kubectl get sc -o yaml
}

run_deploy_caas_workload() {
	echo

	local name k8s_cloud_name model_name file storage controller_name

	name="deploy-caas-workload"
	k8s_cloud_name="k8s-cloud"
	model_name="test-${name}"
	file="${TEST_DIR}/${model_name}.log"

	storage=$(kubectl get sc -o json | jq -r '.items[] | select(.metadata.annotations."storageclass.kubernetes.io/is-default-class"=="true") | .metadata.name')

	controller_name=$(juju controllers --format json | jq -r '.controllers | keys[0]')
	juju add-k8s "${k8s_cloud_name}" --storage "${storage}" --controller "${controller_name}" 2>&1 | OUTPUT "${file}"

	add_model "${model_name}" "${k8s_cloud_name}" "${controller_name}" "${file}"

	juju deploy cs:~juju/mariadb-k8s-3
	juju deploy cs:~juju/mediawiki-k8s-4 --config kubernetes-service-type=loadbalancer
	juju relate mediawiki-k8s:db mariadb-k8s:server

	wait_for "active" '.applications["mariadb-k8s"] | ."application-status".current'
	wait_for "active" '.applications["mediawiki-k8s"] | ."application-status".current'

	destroy_model "${model_name}"
}

test_deploy_ck() {
	if [ "$(skip 'test_deploy_ck')" ]; then
		echo "==> TEST SKIPPED: Test Deploy CK"
		return
	fi

	(
		set_verbosity

		cd .. || exit
		local run_deploy_ck_name="deploy-ck"
		run "run_deploy_ck" "${run_deploy_ck_name}"

		run "run_deploy_caas_workload"

		destroy_model "${run_deploy_ck_name}" 60m
	)
}
