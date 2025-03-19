test_resources() {
	if [ "$(skip 'test_resources')" ]; then
		echo "==> TEST SKIPPED: Resources tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-resources.log"

	bootstrap "test-resources" "${file}"

	test_basic_resources
	test_upgrade_resources

	case "${BOOTSTRAP_PROVIDER:-}" in
	"k8s")
		test_container_resources
		;;
	*)
		echo "==> TEST SKIPPED: test_container_resources - tests for k8s only"
		;;
	esac

	destroy_controller "test-resources"
}
