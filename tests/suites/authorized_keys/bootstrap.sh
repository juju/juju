# run_bootstrap_authorized_keys_loaded is here to test injecting specific public
# keys at bootstrap time into the controller model and not relying on the auto
# load functionality of authorized keys.
#
# What we expect here:
# - The users default ssh keys in their .ssh directory are NOT loaded.
# - Keys specified in authorized_keys are loaded.
# - The key that juju makes in its home directory is loaded.
run_bootstrap_authorized_keys_loaded() {
	SUB_TEST_DIR="${TEST_DIR}/bootstrap_authorized_keys_loaded"
	mkdir -p "${SUB_TEST_DIR}"
	log_file="${SUB_TEST_DIR}/bootstrap.log"

	# Setup a temporary pseudo user ssh directory for Juju to read from.
	ssh_dir="${SUB_TEST_DIR}/.ssh"
	mkdir -p "$ssh_dir"

	# Setup a sudo juju home directory
	juju_home_dir="${SUB_TEST_DIR}/.local/share/juju"

	# Make a default key pair in the user ssh directory.
	default_key_file="${ssh_dir}/id_ed25519"
	default_key_file_pub="${default_key_file}.pub"
	ssh-keygen -t ed25519 -f "$default_key_file" -C "isgreat@juju.is" -P ""

	extra_key_file="${SUB_TEST_DIR}/bootstrap_key"
	extra_key_file_pub="${extra_key_file}.pub"
	ssh-keygen -t ed25519 -f "$extra_key_file" -C "isgreat@juju.is" -P ""

	(
		export HOME="${SUB_TEST_DIR}"
		# Always isolate JUJU_DATA to the subtest directory to avoid leaking/using
		# any CI-provided or globally configured Juju client store.
		export JUJU_DATA="${juju_home_dir}"

		bootstrap_additional_args=(--config "'authorized-keys=$(cat ${extra_key_file_pub})'")
		BOOTSTRAP_ADDITIONAL_ARGS="${bootstrap_additional_args[*]}" \
			BOOTSTRAP_REUSE=false \
			bootstrap "authorized-keys-loaded" "$log_file"
		juju switch controller

		fingerprint=$(ssh-keygen -lf "${extra_key_file_pub}" | cut -f 2 -d ' ')
		check_contains "$(juju ssh-keys)" "$fingerprint"

		fingerprint=$(ssh-keygen -lf "${default_key_file_pub}" | cut -f 2 -d ' ')
		check_not_contains "$(juju ssh-keys)" "$fingerprint"

		# Load the default key made by juju in the juju data directory and make sure
		# that is has automatically been loaded by bootstrap.
		fingerprint=$(ssh-keygen -lf "${JUJU_DATA}/ssh/juju_id_ed25519.pub" | cut -f 2 -d ' ')
		check_contains "$(juju ssh-keys)" "$fingerprint"

		destroy_controller "$BOOTSTRAPPED_JUJU_CTRL_NAME"
	)
}

# run_bootstrap_authorized_keys_default is here to test the default loading of
# authorized keys into the controller model at bootstrap time.
# We expect here:
# - That bootstrap will load the default keys found in the users .ssh directory
# into the controller model.
# - The ssh key that Juju generates in the home/ssh diri to be loaded into the
# controller model
run_bootstrap_authorized_keys_default() {
	SUB_TEST_DIR="${TEST_DIR}/bootstrap_authorized_keys_default"
	mkdir -p "${SUB_TEST_DIR}"
	log_file="${SUB_TEST_DIR}/bootstrap.log"

	# Setup a temporary sudo user ssh directory for Juju to read from
	ssh_dir="${SUB_TEST_DIR}/.ssh"
	mkdir -p "$ssh_dir"

	# Setup a sudo juju home directory
	juju_home_dir="${SUB_TEST_DIR}/.local/share/juju"

	# Make a default key pair in the user ssh directory.
	default_key_file="${ssh_dir}/id_ed25519"
	default_key_file_pub="${default_key_file}.pub"
	ssh-keygen -t ed25519 -f "$default_key_file" -C "isgreat@juju.is" -P ""

	(
		export HOME="${SUB_TEST_DIR}"
		# Always isolate JUJU_DATA to the subtest directory to avoid leaking/using
		# any CI-provided or globally configured Juju client store.
		export JUJU_DATA="${juju_home_dir}"

		BOOTSTRAP_REUSE=false \
			bootstrap "authorized-keys-default" "$log_file"
		juju switch controller

		fingerprint=$(ssh-keygen -lf "${default_key_file_pub}" | cut -f 2 -d ' ')
		check_contains "$(juju ssh-keys)" "$fingerprint"

		# Load the default key made by juju in the juju data directory and make sure
		# that is has automatically been loaded by bootstrap.
		fingerprint=$(ssh-keygen -lf "${JUJU_DATA}/ssh/juju_id_ed25519.pub" | cut -f 2 -d ' ')
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

		run "run_bootstrap_authorized_keys_loaded"
		run "run_bootstrap_authorized_keys_default"
	)
}
