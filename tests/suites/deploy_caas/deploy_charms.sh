# Deploy some simple charms to our k8s controller, and relate them together.
# Then verify that the our application is reachable
run_deploy_charm() {
	echo

	file="${TEST_DIR}/test-deploy-charm.log"

	ensure "test-deploy-charm" "${file}"

	echo "Deploy some charms"
	juju deploy discourse-k8s
	# 14/edge/juju4 is a Juju 4.x compatible branch of postgresql-k8s.
	# The stable/edge channels declare "assumes: juju < 3", so they cannot
	# be deployed on Juju 4.x without --force and a juju4-specific branch.
	juju deploy postgresql-k8s --channel 14/edge/juju4 --trust --base ubuntu@22.04 --force
	juju deploy redis-k8s --channel edge # stable redis is too old
	juju deploy nginx-ingress-integrator
	juju trust nginx-ingress-integrator --scope=cluster
	juju integrate discourse-k8s postgresql-k8s
	juju integrate discourse-k8s redis-k8s
	juju integrate discourse-k8s nginx-ingress-integrator

	wait_for "postgresql-k8s" "$(active_idle_condition "postgresql-k8s")"
	wait_for "redis-k8s" "$(active_idle_condition "redis-k8s")"
	wait_for "discourse-k8s" "$(active_idle_condition "discourse-k8s")"
	wait_for "nginx-ingress-integrator" "$(active_idle_condition "nginx-ingress-integrator")"

	echo "Verify discourse user can be created"
	# discourse-k8s charm introduces a bug, that writes not valid yaml to stdout (injecting WARNING message). Until
	# this is fixed, we can just check that the user is created, by checking that the email is in the output.
	#check_contains "$(juju run discourse-k8s/0 create-user admin=true email=user@example.com | yq .user)" "user@example.com"
	check_contains "$(juju run discourse-k8s/0 create-user admin=true email=user@example.com)" "user: user@example.com"

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
