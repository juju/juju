run_migrate_authorized_keys() {
	SUB_TEST_DIR="${TEST_DIR}/migrate_authorized_keys"
	mkdir -p "${SUB_TEST_DIR}"
	log_file="${SUB_TEST_DIR}/migrate-keys.log"

	# record the controller made before this test
	to_controller=$BOOTSTRAPPED_JUJU_CTRL_NAME

	export BOOTSTRAP_RESUSE=false
	bootstrap "migrate-keys" "$log_file"
	from_controller=$BOOTSTRAPPED_JUJU_CTRL_NAME

	key_file="${SUB_TEST_DIR}/id_ed25519"
	key_file_pub="${key_file}.pub"
	ssh-keygen -t ed25519 -f "$key_file" -C "isgreat@juju.is" -P ""
	fingerprint=$(ssh-keygen -lf "${key_file_pub}" | cut -f 2 -d ' ')

	juju add-ssh-key "$(cat ${key_file_pub})"
	check_contains "$(juju ssh-keys)" "$fingerprint"

	juju migrate "migrate-keys" "${to_controller}"

	juju switch "${to_controller}:controller"

	## Wait for the new model migration to appear in the to controller.
	wait_for_model "migrate-keys"

	juju switch "${to_controller}:migrate-keys"
	check_contains "$(juju ssh-keys)" "$fingerprint"

	destroy_controller "$from_controller"
}

test_migrate_authorized_keys() {
	if [ "$(skip 'test_migrate_authorized_keys')" ]; then
		echo "==> TEST SKIPPED: migrate authorized keys"
		return
	fi

	(
		set_verbosity

		run "run_migrate_authorized_keys"
	)
}
