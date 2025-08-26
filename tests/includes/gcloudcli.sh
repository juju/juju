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
	key_json_file_path=$(cat $HOME/.local/share/juju/credentials.yaml |
		yq e '.credentials.google | to_entries | .[0].value.file')

	if [[ -z $key_json_file_path ]]; then
		echo "Warning: could not determine key json file path" >&2
		return
	fi

	# Activate the service account with the existing json file
	gcloud auth activate-service-account --key-file "$key_json_file_path"
}
