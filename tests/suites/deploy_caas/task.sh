test_deploy_caas() {
	if [ "$(skip 'test_deploy_caas')" ]; then
		echo "==> TEST SKIPPED: Deploy CAAS tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-deploy-caas.log"

	if [[ -n ${OPERATOR_IMAGE_ACCOUNT:-} ]]; then
		export BOOTSTRAP_ADDITIONAL_ARGS="${BOOTSTRAP_ADDITIONAL_ARGS:-} --config caas-image-repo=${OPERATOR_IMAGE_ACCOUNT}"
	fi
	bootstrap "test-deploy-caas" "${file}"

	case "${BOOTSTRAP_PROVIDER:-}" in
	"k8s")
		test_deploy_charm
		;;
	*)
		echo "==> TEST SKIPPED: test_deploy_caas test runs on k8s only"
		;;
	esac

	destroy_controller "test-deploy-caas"
}
