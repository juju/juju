# Deploy some simple charms to our AKS k8s controller, and relate them together.
# Then verify that the our application is reachable
run_deploy_aks_charms() {
	echo

	echo "Add model"
	juju add-model "test-deploy-aks-charms"

	echo "Deploy some charms"
	juju deploy postgresql-k8s --channel 14/stable
	#	Current Mattermost charm will crash (container crash) if related to new postgres charm (from edge)
	#
	# That is because new psql does not have TLS built-in. Fix is simple, integrate psql with certificates charm.
	# However, MM should not have container crash but go to the blocked state with clear error message to a user until TLS is available.
	# Thx to https://bugs.launchpad.net/charm-k8s-mattermost/+bug/1997540
	juju deploy mattermost-k8s
	juju deploy tls-certificates-operator --channel=edge
	juju config tls-certificates-operator generate-self-signed-certificates="true" ca-common-name="Test CA"

	juju relate postgresql-k8s tls-certificates-operator
	# For postgresql-k8s the "db" interface needs to be specified as the charm provides more than one
	juju relate mattermost-k8s postgresql-k8s:db

	wait_for "tls-certificates-operator" "$(idle_condition "tls-certificates-operator" 2)"
	wait_for "postgresql-k8s" "$(idle_condition "postgresql-k8s" 1)"
	wait_for "mattermost-k8s" "$(idle_condition "mattermost-k8s" 0)"

	echo "Verify application is reachable"
	mattermost_ip="$(juju status --format json | jq -r '.applications["mattermost-k8s"].units["mattermost-k8s/0"].address')"
	curl "${mattermost_ip}:8065/api/v4/system/ping" >/dev/null

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
