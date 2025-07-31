# Deploy some simple charms to our AKS k8s controller, and relate them together.
# Then verify that the our application is reachable
run_deploy_aks_charms() {
	echo

	echo "Add model"
	juju add-model "test-deploy-aks-charms"

	echo "Deploy some charms"
	juju deploy juju-qa-dummy-sink --base ubuntu@22.04
	juju deploy juju-qa-dummy-source --base ubuntu@22.04

	juju relate dummy-sink dummy-source

	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"
	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	echo "Verify application"
	juju config dummy-source token=yeah-boi
	wait_for "yeah-boi" "$(workload_status "dummy-sink" 0).message"

	echo "Destroy model"
	juju destroy-model "test-deploy-aks-charms" --destroy-storage -y
}

test_deploy_aks_charms() {
	if [ "$(skip 'test_deploy_aks_charms')" ]; then
		echo "==> TEST SKIPPED: Test deploy aks charms"
		return
	fi
	(
		set_verbosity

		cd .. || exit

		run "run_deploy_aks_charms"
	)
}
