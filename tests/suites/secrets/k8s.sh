run_secrets_k8s() {
	# TODO
	echo

	file="${TEST_DIR}/model-secrets-juju.txt"

	ensure "model-secrets-juju" "${file}"

	destroy_model "model-secrets-juju"
}

test_secrets_k8s() {
	if [ "$(skip 'test_secrets_k8s')" ]; then
		echo "==> TEST SKIPPED: test_secrets_k8s"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_secrets_k8s"
	)
}
