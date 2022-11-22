# Run the canonical/kubeflow-ci integration test suite from a stable channel of kubeflow
# with our build of Juju under test
run_deploy_kubeflow() {
	echo

	file="${TEST_DIR}/deploy-kubeflow.txt"

	# charmed-kubeflow must be deployed to a namespace called 'kubeflow'
	# https://git.io/J6d35
	ensure "kubeflow" "${file}"

	echo "==> Installing tox"
	pip3 install tox

	echo "==> Cloning ci repo"
	kubeflow_ci_dir=$(mktemp -d)
	git clone "https://github.com/canonical/kubeflow-ci" "${kubeflow_ci_dir}"

	echo "==> Running CI"
	cd "${kubeflow_ci_dir}" || exit
	python3 -m tox -e test_1dot6 -- --channel=1.6/stable
}

test_deploy_kubeflow() {
	if [ "$(skip 'test_deploy_kubeflow')" ]; then
		echo "==> TEST SKIPPED: test_deploy_kubeflow"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_deploy_kubeflow"
	)
}
