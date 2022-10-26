run_secrets_vault() {
	# TODO
	echo

	file="${TEST_DIR}/model-secrets-juju.txt"

	ensure "model-secrets-juju" "${file}"

	destroy_model "model-secrets-juju"
}

test_secrets_vault() {
	if [ "$(skip 'test_secrets_vault')" ]; then
		echo "==> TEST SKIPPED: test_secrets_vault"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_secrets_vault"
	)
}
