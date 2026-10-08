setup_awscli_credential() {
	: "${CREDS_DIR:?CREDS_DIR must be set}"

	if ! which aws >/dev/null 2>&1; then
		sudo snap install aws-cli --classic || true
	fi

	# AWS_DEFAULT_PROFILE, AWS_SHARED_CREDENTIALS_FILE and AWS_CONFIG_FILE
	# are exported from tests/main.sh next to CREDS_DIR, not here, so that
	# cleanup_funcs registered via add_clean_func (which run from
	# cleanup() in the parent shell, not this suite's subshell) also see
	# them when tearing down AWS resources created during the test. This
	# isolates the AWS CLI config under CREDS_DIR instead of writing to
	# $HOME/.aws, so a test run never clobbers a developer's own AWS
	# credentials; CREDS_DIR lives outside TEST_DIR and is removed
	# unconditionally in main.sh's cleanup, regardless of test result, so
	# these keys are never archived by archive_logs and never retained
	# alongside TEST_DIR when a failed run is kept around for debugging.
	if [ -f "${AWS_SHARED_CREDENTIALS_FILE}" ] && [ -f "${AWS_CONFIG_FILE}" ]; then
		return
	fi

	mkdir -p "${CREDS_DIR}/aws"
	echo "[default]" >"${AWS_SHARED_CREDENTIALS_FILE}"
	cat "$HOME/.local/share/juju/credentials.yaml" |
		grep aws: -A 4 | grep key: |
		tail -2 |
		sed -e 's/      access-key:/aws_access_key_id =/' \
			-e 's/      secret-key:/aws_secret_access_key =/' \
			>>"${AWS_SHARED_CREDENTIALS_FILE}"
	echo -e "[default]\nregion = us-east-1" >"${AWS_CONFIG_FILE}"
	chmod 600 "${CREDS_DIR}"/aws/*
}
