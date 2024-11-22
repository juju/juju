run_deploy_kubeflow() {
	echo

	file="${TEST_DIR}/deploy-kubeflow.txt"

	# charmed-kubeflow must be deployed to a namespace called 'kubeflow'
	# https://git.io/J6d35
	ensure "kubeflow" "${file}"

	echo "==> Deploying kubeflow"
	juju deploy kubeflow --trust --channel 1.9

	echo "==> Checking kubeflow deployment"
	num_apps=$(juju status --format json | jq '.applications | length')
	wait_for "training-operator" "$(active_idle_condition "training-operator" $((num_apps - 1)))" 1800
	jupyter_ip=$(microk8s kubectl -n kubeflow get svc istio-ingressgateway-workload -o jsonpath='{.status.loadBalancer.ingress[0].ip}')

	attempt=0
	# shellcheck disable=SC2046,SC2143,SC2091,SC2086
	until $(check_contains "$(curl ${jupyter_ip})" "Found" >/dev/null 2>&1); do
		echo "[+] (attempt ${attempt}) jupyter ui"
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))

		if [[ ${attempt} -gt 10 ]]; then
			echo "failed waiting for jupyter ui"
			exit 1
		fi
	done
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
