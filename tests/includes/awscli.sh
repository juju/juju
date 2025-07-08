setup_awscli_credential() {
	if ! which aws >/dev/null 2>&1; then
		sudo snap install aws-cli --classic || true
	fi

	export AWS_DEFAULT_PROFILE=default
	if [ -f "$HOME/.aws/credentials" ]; then
		return
	fi

	mkdir -p "$HOME"/.aws
	echo "[default]" >"$HOME/.aws/credentials"
	cat "$HOME/.local/share/juju/credentials.yaml" |
		grep aws: -A 4 | grep key: |
		tail -2 |
		sed -e 's/      access-key:/aws_access_key_id =/' \
			-e 's/      secret-key:/aws_secret_access_key =/' \
			>>"$HOME/.aws/credentials"
	echo -e "[default]\nregion = us-east-1" >"$HOME/.aws/config"
	chmod 600 $HOME/.aws/*
}
