run_constraints_k8s() {
	name="constraints-k8s"

	file="${TEST_DIR}/constraints-k8s.txt"
	ensure "${name}" "${file}"

	juju set-model-constraints "mem=2G"
	juju deploy snappass-test --constraints "cpu-power=200"
	wait_for "snappass-test" "$(idle_condition "snappass-test")"

	application_constraints=$(juju constraints snappass-test)
	check_contains "${application_constraints}" "arch="
	check_contains "${application_constraints}" "cpu-power=200"
	check_not_contains "${application_constraints}" "mem="

	resources=$(kubectl -n "${name}" get pod snappass-test-0 -o json |
		yq -r '.spec.containers[] | select(.name != "charm") | .resources.limits.cpu')
	check_contains "${resources}" '200m'
	resources=$(kubectl -n "${name}" get pod snappass-test-0 -o json |
		yq -r '.spec.containers[] | select(.name != "charm") | .resources.limits.memory')
	check_contains "${resources}" '2Gi'

	echo "Update the application constraints"
	juju set-constraints snappass-test "cpu-power=300 mem=2G"

	application_constraints=$(juju constraints snappass-test)
	check_contains "${application_constraints}" "cpu-power=300"
	check_contains "${application_constraints}" "mem=2048M"
	check_not_contains "${application_constraints}" "arch="

	echo "Scale the application to apply the updated constraints to a new unit"
	juju add-unit snappass-test
	wait_for "snappass-test" "$(idle_condition "snappass-test" 1)"

	resources=$(kubectl -n "${name}" get pod snappass-test-1 -o json |
		yq -r '.spec.containers[] | select(.name != "charm") | .resources.limits.cpu')
	check_contains "${resources}" '300m'
	resources=$(kubectl -n "${name}" get pod snappass-test-1 -o json |
		yq -r '.spec.containers[] | select(.name != "charm") | .resources.limits.memory')
	check_contains "${resources}" '2Gi'

	destroy_model "${name}"
}
