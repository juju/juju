test_caasadmission() {
	if [ "$(skip 'test_caasadmission')" ]; then
		echo "==> TEST SKIPPED: caas admission tests"
		return
	fi

	if [[ -n ${OPERATOR_IMAGE_ACCOUNT:-} ]]; then
		export BOOTSTRAP_ADDITIONAL_ARGS="${BOOTSTRAP_ADDITIONAL_ARGS:-} --config caas-image-repo=${OPERATOR_IMAGE_ACCOUNT}"
	fi

	set_verbosity

	case "${BOOTSTRAP_PROVIDER:-}" in
	"k8s")
		echo "==> Checking for dependencies"
		check_dependencies petname

		microk8s config >"${TEST_DIR}"/kube.conf
		export KUBE_CONFIG="${TEST_DIR}"/kube.conf

		test_controller_model_admission
		test_new_model_admission
		test_model_chicken_and_egg
		;;
	*)
		echo "==> TEST SKIPPED: caas admission tests, not a k8s provider"
		;;
	esac
}
