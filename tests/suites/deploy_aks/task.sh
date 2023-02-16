test_deploy_caas() {
	if [ "$(skip 'test_deploy_caas')" ]; then
		echo "==> TEST SKIPPED: Deploy CAAS tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	az group create -l eastus -n test-aks-resource-group
	az aks create -g test-aks-resource-group -n aks-cluster --generate-ssh-keys
	juju add-k8s --aks --client --resource-group test-aks-resource-group --storage test-aks-storage --cluster-name aks-cluster aks-k8s-cloud

	bootstrap_custom_controller "aks" "aks-k8s-cloud"

	case "${BOOTSTRAP_PROVIDER:-}" in
	"k8s")
		test_deploy_aks
		;;
	*)
		echo "==> TEST SKIPPED: test_deploy_caas test runs on k8s only"
		;;
	esac

	juju destroy-controller aks --destroy-all-models --destroy-storage -y
	juju remove-k8s --client aks-k8s-cloud
	az group delete -y -g test-aks-resource-group
}
