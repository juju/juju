run_model_defaults_isomorphic() {
	echo

	FILE=$(mktemp)

	juju model-defaults --cloud "${BOOTSTRAPPED_CLOUD}" --format=yaml | juju model-defaults --cloud "${BOOTSTRAPPED_CLOUD}" --ignore-read-only-fields --file -
}

run_model_defaults_cloudinit_userdata() {
	echo

	FILE=$(mktemp)

	cat <<EOF >"${FILE}"
cloudinit-userdata: |
  packages:
    - shellcheck
EOF

	juju model-defaults --cloud "${BOOTSTRAPPED_CLOUD}" --file "${FILE}"
	juju model-defaults --cloud "${BOOTSTRAPPED_CLOUD}" cloudinit-userdata --format=yaml | grep -q 'default: ""'
	juju model-defaults --cloud "${BOOTSTRAPPED_CLOUD}" cloudinit-userdata --format=yaml | grep -q "shellcheck"
}

run_model_defaults_boolean() {
	echo

	juju model-defaults --cloud "${BOOTSTRAPPED_CLOUD}" automatically-retry-hooks --format=json | jq '."automatically-retry-hooks"."default"' | grep '^true$'
	juju model-defaults --cloud "${BOOTSTRAPPED_CLOUD}" automatically-retry-hooks=false
	juju model-defaults --cloud "${BOOTSTRAPPED_CLOUD}" automatically-retry-hooks --format=json | jq '."automatically-retry-hooks"."controller"' | grep '^false$'
	juju model-defaults --cloud "${BOOTSTRAPPED_CLOUD}" | grep -E 'automatically-retry-hooks +true +false'
	juju model-defaults --cloud "${BOOTSTRAPPED_CLOUD}" automatically-retry-hooks --format=yaml | grep 'default: true'
	juju model-defaults --cloud "${BOOTSTRAPPED_CLOUD}" automatically-retry-hooks --format=yaml | grep 'controller: false'
}

run_model_defaults_region_aws() {
	echo

	juju model-defaults --cloud "${BOOTSTRAPPED_CLOUD}" --format=json test-mode | jq '."test-mode"."default"'
	juju model-defaults --cloud "${BOOTSTRAPPED_CLOUD}" --format=yaml aws/ca-central-1 test-mode=true
	juju model-defaults --cloud "${BOOTSTRAPPED_CLOUD}" --format=json aws/ca-central-1 test-mode | jq '."test-mode".regions[0].value' | grep '^true$'
	juju model-defaults --cloud "${BOOTSTRAPPED_CLOUD}" --format=json test-mode | jq '."test-mode".regions[]|select(.name=="ca-central-1").value' | grep '^true$'
}

test_model_defaults() {
	if [ "$(skip 'test_model_defaults')" ]; then
		echo "==> TEST SKIPPED: model defaults"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		# save model-defaults
		SAVED_DEFAULTS_FILE=$(mktemp)
		juju model-defaults --cloud "${BOOTSTRAPPED_CLOUD}" --format=yaml >"${SAVED_DEFAULTS_FILE}"

		run "run_model_defaults_isomorphic"
		run "run_model_defaults_cloudinit_userdata"
		run "run_model_defaults_boolean"

		case "${BOOTSTRAP_PROVIDER-}" in
		"ec2")
			run "run_model_defaults_region_aws"
			;;
		*)
			echo "==> TEST SKIPPED: run_model_defaults_region_aws runs on AWS only"
			;;
		esac

		# restore model-defaults
		juju model-defaults --cloud "${BOOTSTRAPPED_CLOUD}" --ignore-read-only-fields --file "${SAVED_DEFAULTS_FILE}"
	)
}
