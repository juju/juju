setup_gcloudcli_credential() {
	if ! command -v gcloud >/dev/null 2>&1; then
		if ! sudo snap install google-cloud-cli --classic; then
			echo "Error: failed to install google-cloud-cli snap" >&2
		fi
	fi

	# Check if a service account is already active
	if gcloud auth list --filter="status:ACTIVE" \
		--format="value(account)" | grep -q "gserviceaccount\.com$"; then
		return
	fi

	local key_json_file_path

	google_entry=$(cat "$HOME/.local/share/juju/credentials.yaml" | yq e '.credentials.google | to_entries | .[0].value' -)
	#	The `file` field points to a JSON file, which contains the private key.
	key_json_file_path=$(echo "$google_entry" | yq e '.file' -)

	# If credentials.yaml doesn't have a `file` field
	# we assume that this yaml file has the contents expanded so we read from it.
	if [[ $key_json_file_path == "null" || -z $key_json_file_path ]]; then
		tmp_key_file=$(mktemp /tmp/google-key.XXXXXX.json)
		echo "$google_entry" |
			yq e '.. | select(tag == "!!map") | with_entries(.key |= sub("-"; "_"))' -o=json - \
				>"$tmp_key_file"
		key_json_file_path="$tmp_key_file"
	fi

	gcloud auth activate-service-account --key-file "$key_json_file_path"
}
