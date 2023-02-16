test_deploy_aks() {
	if [ "$(skip 'test_deploy_aks')" ]; then
		echo "==> TEST SKIPPED: Deploy aks tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	az group create -l eastus -n test-aks-resource-group
	az aks create -g test-aks-resource-group -n aks-cluster --generate-ssh-keys
	juju add-k8s --aks --client --resource-group test-aks-resource-group --storage test-aks-storage --cluster-name aks-cluster aks-k8s-cloud

	bootstrap_custom_controller "test-deploy-aks" "aks-k8s-cloud"

	case "${BOOTSTRAP_PROVIDER:-}" in
	"k8s")
		test_deploy_aks_charms
		;;
	*)
		echo "==> TEST SKIPPED: test_deploy_aks_charms test runs on k8s only"
		;;
	esac

	destroy_controller "test-deploy-aks"

	juju remove-k8s --client aks-k8s-cloud
	az group delete -y -g test-aks-resource-group
}
