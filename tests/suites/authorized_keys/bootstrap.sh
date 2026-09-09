# run_bootstrap_authorized_keys_loaded is here to test injecting specific public
# keys at bootstrap time into the controller model.
#
# What we expect here:
# - Keys specified in authorized_keys are loaded.
run_bootstrap_authorized_keys_loaded() {
	SUB_TEST_DIR="${TEST_DIR}/bootstrap_authorized_keys_loaded"
	mkdir -p "${SUB_TEST_DIR}"
	log_file="${SUB_TEST_DIR}/bootstrap.log"

	# Setup a sudo juju home directory
	juju_home_dir="${SUB_TEST_DIR}/.local/share/juju"

	extra_key_file="${SUB_TEST_DIR}/bootstrap_key"
	extra_key_file_pub="${extra_key_file}.pub"
	ssh-keygen -t ed25519 -f "$extra_key_file" -C "isgreat@juju.is" -P ""

	(
		export HOME="${SUB_TEST_DIR}"
		export JUJU_DATA="${JUJU_DATA:=$juju_home_dir}"

		bootstrap_additional_args=(--config "'authorized-keys=$(cat ${extra_key_file_pub})'")
		BOOTSTRAP_ADDITIONAL_ARGS="${bootstrap_additional_args[*]}" \
			BOOTSTRAP_REUSE=false \
			bootstrap "authorized-keys-loaded" "$log_file"
		juju switch controller

		fingerprint=$(ssh-keygen -lf "${extra_key_file_pub}" | cut -f 2 -d ' ')
		check_contains "$(juju ssh-keys)" "$fingerprint"

		destroy_controller "$BOOTSTRAPPED_JUJU_CTRL_NAME"
	)
}

test_bootstrap_authorized_keys() {
	if [ "$(skip 'test_bootstrap_authorized_keys')" ]; then
		echo "==> TEST SKIPPED: bootstrap with authorized keys"
		return
	fi

	(
		set_verbosity

		# The following tests bootstrap a new controller, make sure to switch back to the
		# previous controller when we are done so that the test-runner can clean things up.
		current_controller=$(juju controllers --format json | yq -r '."current-controller"')

		run "run_bootstrap_authorized_keys_loaded"

		juju switch "$current_controller"
	)
}
