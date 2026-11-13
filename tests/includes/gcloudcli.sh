setup_gcloudcli_credential() {
	: "${CREDS_DIR:?CREDS_DIR must be set}"

	if ! command -v gcloud >/dev/null 2>&1; then
		if ! sudo snap install google-cloud-cli --classic; then
			echo "Error: failed to install google-cloud-cli snap" >&2
		fi
	fi

	# CLOUDSDK_CONFIG is exported from tests/main.sh next to CREDS_DIR,
	# not here, so that cleanup_funcs registered via add_clean_func
	# (which run from cleanup() in the parent shell, not this suite's
	# subshell) also see it when tearing down GCE resources created
	# during the test. This isolates the gcloud CLI config under
	# CREDS_DIR instead of the developer's real ~/.config/gcloud, so a
	# test run never clobbers real gcloud state; CREDS_DIR lives outside
	# TEST_DIR and is removed unconditionally in main.sh's cleanup,
	# regardless of test result, so these credentials are never archived
	# and never retained alongside a kept TEST_DIR.

	# Check if an account is already active in this run. CREDS_DIR is
	# fresh per invocation of main.sh, so this only guards against
	# re-activating within the same run across suites, not against the
	# developer's own prior gcloud state.
	if [[ "$(gcloud config get account 2>&1)" != "(unset)" ]]; then
		return
	fi

	local key_json_file_path

	google_entry=$(cat "$HOME/.local/share/juju/credentials.yaml" | yq '.credentials.google | to_entries | .[0].value')
	#	The `file` field points to a JSON file, which contains the private key.
	key_json_file_path=$(echo "$google_entry" | yq '.file // ""')

	# If credentials.yaml doesn't have a `file` field
	# we assume that this yaml file has the contents expanded so we read from it.
	if [[ -z $key_json_file_path ]]; then
		# Written under CREDS_DIR, not /tmp, so the unconditional
		# rm -rf "${CREDS_DIR}" in main.sh's cleanup sweeps this key
		# material too instead of leaving it behind indefinitely.
		tmp_key_file=$(mktemp "${CREDS_DIR}/google-key.XXXXXX.json")
		echo "$google_entry" |
			yq -o=json '.. | select(tag == "!!map") | with_entries(.key |= sub("-"; "_"))' \
				>"$tmp_key_file"
		key_json_file_path="$tmp_key_file"
	fi

	gcloud auth activate-service-account --key-file "$key_json_file_path"

	project_id=$(yq -r '.project_id' "$key_json_file_path")
	gcloud config set project "$project_id"
}
