test_deploy_aks() {
	if [ "$(skip 'test_deploy_aks')" ]; then
		echo "==> TEST SKIPPED: Deploy aks tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	resource_group_name="test-aks-resource-group-$(rnd_str)"
	echo "rname = ${resource_group_name}"
	az group create -l eastus -n "${resource_group_name}"
	az aks create -g "${resource_group_name}" -n aks-cluster --generate-ssh-keys
	juju add-k8s --aks --client --resource-group "${resource_group_name}" --storage test-aks-storage --cluster-name aks-cluster aks-k8s-cloud

	bootstrap_custom_controller "test-deploy-aks" "aks-k8s-cloud"

	test_deploy_aks_charms

	destroy_controller "test-deploy-aks"

	juju remove-k8s --client aks-k8s-cloud
	az group delete -y -g "${resource_group_name}"
}
