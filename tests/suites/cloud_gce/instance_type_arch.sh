run_instance_type_arch() {
	echo
	file="${TEST_DIR}/test-instance-type-arch.log"
	ensure "test-instance-type-arch" "${file}"

	# c4a is an ARM machine type and the model's agents are amd64. Juju must
	# reject the pair itself, rather than ask GCE for an ARM machine with an
	# amd64 boot disk.
	juju add-machine --constraints instance-type=c4a-highcpu-2
	wait_for "no instance types in .* matching constraints" '.machines["0"]["machine-status"].message'

	destroy_model "test-instance-type-arch"
}

test_instance_type_arch() {
	if [ "$(skip 'test_instance_type_arch')" ]; then
		echo "==> TEST SKIPPED: instance type arch"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_instance_type_arch"
	)
}
