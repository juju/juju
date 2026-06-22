run_constraints_k8s() {
	name="constraints-k8s"

	file="${TEST_DIR}/constraints-k8s.txt"
	ensure "${name}" "${file}"

	juju deploy snappass-test --constraints "mem=2G cpu-power=200"
	wait_for "snappass-test" "$(idle_condition "snappass-test")"

	resources=$(kubectl -n "${name}" get pod snappass-test-0 -o json |
		yq -r '.spec.containers[] | select(.name != "charm") | .resources')

	check_contains "${resources}" 'cpu: 200m'
	check_contains "${resources}" 'memory: 2Gi'

	destroy_model "${name}"
}
