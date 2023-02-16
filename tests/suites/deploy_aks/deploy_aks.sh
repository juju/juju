# Deploy some simple charms to our AKS k8s controller, and relate them together.
# Then verify that the our application is reachable
run_deploy_aks() {
	echo

	file="${TEST_DIR}/test-deploy-aks.log"

	echo "Add model"
	juju add-model "test-deploy-aks"

	echo "Deploy some charms"
	juju deploy postgresql-k8s
	juju deploy mattermost-k8s
	juju relate mattermost-k8s postgresql-k8s:db

	wait_for "postgresql-k8s" "$(idle_condition "postgresql-k8s" 1)"
	wait_for "mattermost-k8s" "$(idle_condition "mattermost-k8s" 0)"

	echo "Verify application is reachable"
	mattermost_ip="$(juju status --format json | jq -r '.applications["mattermost-k8s"].units["mattermost-k8s/0"].address')"
	juju run --unit mattermost-k8s/0 "curl ${mattermost_ip}:8065 >/dev/null"

	echo "Destroy model"
	juju destroy-model "test-deploy-aks" -y
}

test_deploy_aks() {
	if [ "$(skip 'test_deploy_aks')" ]; then
		echo "==> TEST SKIPPED: Test deploy aks"
		return
	fi
	(
		set_verbosity

		cd .. || exit

		run "run_deploy_aks"
	)
}
