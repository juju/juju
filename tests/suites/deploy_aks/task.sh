test_deploy_aks() {
	# TODO(wallyworld): we need to fix the az cli account login
	# shellcheck disable=SC2160
	if [ true ]; then
		echo "==> TEST SKIPPED: Deploy aks tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	# TODO(anvial): we need to add separate provider for such tests (for example, "aks") and move all this code to
	# create/delete to code from here.
	resource_group_name="juju-test-aks-resource-group-$(rnd_str)"
	az group create -l eastus -n "${resource_group_name}"
	az aks create -g "${resource_group_name}" -n aks-cluster --generate-ssh-keys
	juju add-k8s --aks --client --resource-group "${resource_group_name}" --storage test-aks-storage --cluster-name aks-cluster aks-k8s-cloud

	export BOOTSTRAP_PROVIDER=k8s
	bootstrap_custom_controller "test-deploy-aks" "aks-k8s-cloud"

	test_deploy_aks_charms

	destroy_controller "test-deploy-aks"

	juju remove-k8s --client aks-k8s-cloud
	az group delete -y -g "${resource_group_name}"
}
