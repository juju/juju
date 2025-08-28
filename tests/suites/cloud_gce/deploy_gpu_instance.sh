run_deploy_gpu_instance() {
	echo

	file="${TEST_DIR}/test-deploy-gpu-instance.log"

	ensure "test-deploy-gpu-instance" "${file}"

	juju deploy ubuntu --base ubuntu@24.04 --constraints instance-type=g2-standard-4
	wait_for_machine_agent_status "0" "started"

	destroy_model "test-deploy-gpu-instance"
}

test_deploy_gpu_instance() {
	if [ "$(skip 'test_deploy_gpu_instance')" ]; then
		echo "==> TEST SKIPPED: deploy gpu instance"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_deploy_gpu_instance"
	)
}
