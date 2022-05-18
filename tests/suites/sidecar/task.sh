test_sidecar() {
	if [ "$(skip 'test_sidecar')" ]; then
		echo "==> TEST SKIPPED: sidecar charm tests"
		return
	fi

	if [[ -n ${OPERATOR_IMAGE_ACCOUNT:-} ]]; then
		export BOOTSTRAP_ADDITIONAL_ARGS="${BOOTSTRAP_ADDITIONAL_ARGS:-} --config caas-image-repo=${OPERATOR_IMAGE_ACCOUNT}"
	fi

	set_verbosity

	case "${BOOTSTRAP_PROVIDER:-}" in
	"k8s")
		test_deploy_and_remove_application
		test_deploy_and_force_remove_application
		;;
	*)
		echo "==> TEST SKIPPED: sidecar charm tests, not a k8s provider"
		;;
	esac
}
