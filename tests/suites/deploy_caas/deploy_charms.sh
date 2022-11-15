# Deploy some simple charms to our k8s controller, and relate them together.
# Then verify that the our application is reachable
run_deploy_charm() {
	echo

	file="${TEST_DIR}/test-deploy-charm.log"

	ensure "test-deploy-charm" "${file}"

	echo "Deploy some charms"
	juju deploy postgresql-k8s
	juju deploy mattermost-k8s
	juju relate mattermost-k8s postgresql-k8s:db

	wait_for "postgresql-k8s" "$(idle_condition "postgresql-k8s" 1)"
	wait_for "mattermost-k8s" "$(idle_condition "mattermost-k8s" 0)"

	echo "Verify application is reachable"
	mattermost_ip="$(juju status --format json | jq -r '.applications["mattermost-k8s"].units["mattermost-k8s/0"].address')"
	curl "${mattermost_ip}:8065" >/dev/null

	destroy_model "test-deploy-charm"
}

test_deploy_charm() {
	if [ "$(skip 'test_deploy_charm')" ]; then
		echo "==> TEST SKIPPED: Test deploy charm"
		return
	fi
	(
		set_verbosity

		cd .. || exit

		run "run_deploy_charm"
	)
}
