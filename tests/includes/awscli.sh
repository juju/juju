setup_awscli_credential() {
	: "${TEST_DIR:?TEST_DIR must be set}"

	if ! which aws >/dev/null 2>&1; then
		sudo snap install aws-cli --classic || true
	fi

	export AWS_DEFAULT_PROFILE=default
	# Isolate the AWS CLI config under TEST_DIR instead of writing to
	# $HOME/.aws, so a test run never clobbers a developer's own AWS
	# credentials. TEST_DIR is removed on a successful run and kept
	# around on failure for debugging; either way, archive_logs in
	# main.sh excludes the aws/ subdirectory from the artifact tarball
	# so these keys are never shipped in a CI artifact.
	export AWS_SHARED_CREDENTIALS_FILE="${TEST_DIR}/aws/credentials"
	export AWS_CONFIG_FILE="${TEST_DIR}/aws/config"
	if [ -f "${AWS_SHARED_CREDENTIALS_FILE}" ] && [ -f "${AWS_CONFIG_FILE}" ]; then
		return
	fi

	mkdir -p "${TEST_DIR}/aws"
	echo "[default]" >"${AWS_SHARED_CREDENTIALS_FILE}"
	cat "$HOME/.local/share/juju/credentials.yaml" |
		grep aws: -A 4 | grep key: |
		tail -2 |
		sed -e 's/      access-key:/aws_access_key_id =/' \
			-e 's/      secret-key:/aws_secret_access_key =/' \
			>>"${AWS_SHARED_CREDENTIALS_FILE}"
	echo -e "[default]\nregion = us-east-1" >"${AWS_CONFIG_FILE}"
	chmod 600 "${TEST_DIR}"/aws/*
}
