test_ovs_maas() {
	if [ "$(skip 'test_ovs_maas')" ]; then
		echo "==> TEST SKIPPED: OVS tests (MAAS)"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-ovs-maas.log"

	# shellcheck disable=SC2140
	export BOOTSTRAP_ADDITIONAL_ARGS="${BOOTSTRAP_ADDITIONAL_ARGS:-} --bootstrap-base=ubuntu@22.04 --bootstrap-constraints="tags=ovs""
	bootstrap "test-ovs-maas" "${file}"

	test_ovs_netplan_config

	destroy_controller "test-ovs-maas"
}
