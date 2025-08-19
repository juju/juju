run_pro_images() {
	echo

	file="${TEST_DIR}/test-pro-images.log"
	ensure "test-pro-images" "${file}"

	juju model-config image-stream=pro
	juju metadata add-image --base ubuntu@24.04 ubuntu-pro-2404-noble-amd64-v20250805 --stream pro
	juju deploy ubuntu
	wait_for "ubuntu" "$(idle_condition "ubuntu")"
	pro_status=$(juju ssh 0 'sudo pro status --format json | jq -r .attached')
	check_contains "$pro_status" "true"

	destroy_model "test-pro-images"
}

test_pro_images() {
	if [ "$(skip 'test_pro_images')" ]; then
		echo "==> TEST SKIPPED: pro images"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_pro_images" "$@"
	)
}
