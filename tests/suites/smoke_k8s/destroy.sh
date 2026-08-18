run_deploy_and_destroy_controller() {
	echo

	file="${2}"

	ensure "test-deploy-destroy" "${file}"

	juju deploy snappass-test --revision 8 --channel stable
	wait_for "snappass-test" "$(idle_condition "snappass-test")"

	# Do not destroy the model: the controller teardown must reclaim the
	# hosted model and its namespace itself.
	destroy_controller "${BOOTSTRAPPED_JUJU_CTRL_NAME}"

	# kill-controller is the forceful escape hatch and does not wait for
	# hosted model removal, so namespaces may legitimately remain.
	if [[ ${KILL_CONTROLLER:-} == "true" ]]; then
		echo "====> KILL_CONTROLLER set, skipping namespace checks"
		return
	fi

	wait_for_namespace_gone "test-deploy-destroy"
	wait_for_namespace_gone "controller-${BOOTSTRAPPED_JUJU_CTRL_NAME}"
}

test_destroy_controller() {
	if [ "$(skip 'test_destroy_controller')" ]; then
		echo "==> TEST SKIPPED: k8s destroy controller tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		file="${1}"

		run "run_deploy_and_destroy_controller" "${file}"
	)
}
