run_model_defaults_isomorphic() {
	echo

	FILE=$(mktemp)

	juju model-defaults --format=yaml | juju model-defaults --ignore-read-only-fields -
}

run_model_defaults_cloudinit_userdata() {
	echo

	FILE=$(mktemp)

	cat <<EOF >"${FILE}"
cloudinit-userdata: |
  packages:
    - shellcheck
EOF

	juju model-defaults "${FILE}"
	juju model-defaults cloudinit-userdata --format=yaml | grep -q 'default: ""'
	juju model-defaults cloudinit-userdata --format=yaml | grep -q "shellcheck"
}

run_model_defaults_boolean() {
	echo

	juju model-defaults automatically-retry-hooks --format=json | jq '."automatically-retry-hooks"."default"' | grep '^true$'
	juju model-defaults automatically-retry-hooks=false
	juju model-defaults automatically-retry-hooks --format=json | jq '."automatically-retry-hooks"."controller"' | grep '^false$'
	juju model-defaults | grep -E 'automatically-retry-hooks +true +false'
	juju model-defaults automatically-retry-hooks --format=yaml | grep 'default: true'
	juju model-defaults automatically-retry-hooks --format=yaml | grep 'controller: false'
}

run_model_defaults_region() {
	echo

	juju model-defaults --format=json test-mode | jq '."test-mode"."default"'
	juju model-defaults --format=yaml aws/ca-central-1 test-mode=true
	juju model-defaults --format=json aws/ca-central-1 test-mode | jq '."test-mode".regions[0].value' | grep '^true$'
	juju model-defaults --format=json test-mode | jq '."test-mode".regions[]|select(.name=="ca-central-1").value' | grep '^true$'
}

test_model_defaults() {
	if [ "$(skip 'test_model_defaults')" ]; then
		echo "==> TEST SKIPPED: model defaults"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_model_defaults_isomorphic"
		run "run_model_defaults_cloudinit_userdata"
		run "run_model_defaults_boolean"

		case "${BOOTSTRAP_PROVIDER:-}" in
		"aws")
			run "run_model_defaults_region"
			;;
		*)
			echo "==> TEST SKIPPED: run_model_defaults_region for AWS only"
			;;
		esac
	)
}
