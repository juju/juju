# Deploy some simple charms to our AKS k8s controller, and relate them together.
# Then verify that the our application is reachable
run_deploy_aks_charms() {
	echo

	echo "Add model"
	juju add-model "test-deploy-aks-charms"

	echo "Deploy some charms"
	juju deploy postgresql-k8s
	juju deploy mattermost-k8s
	juju relate mattermost-k8s postgresql-k8s:db

	wait_for "postgresql-k8s" "$(idle_condition "postgresql-k8s" 1)"
	wait_for "mattermost-k8s" "$(idle_condition "mattermost-k8s" 0)"

	# TODO(anvial): we can return this check after fixing issue with flapping connection by curl in aks environment.
	#	echo "Verify application is reachable"
	#	mattermost_ip="$(juju status --format json | jq -r '.applications["mattermost-k8s"].units["mattermost-k8s/0"].address')"
	#	juju run --unit mattermost-k8s/0 curl "${mattermost_ip}:8065"

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
