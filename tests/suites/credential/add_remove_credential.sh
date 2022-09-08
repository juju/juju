# Adds and removes credentials from file to our juju client.
run_add_remove_credential() {
	# Echo out to ensure nice output to the test suite.
	echo

	model_name="test-add-credential"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju add-credential aws -f ./tests/suites/credential/credentials-data/fake-credentials.yaml --client

	juju credentials aws --format=json | jq -r '."client-credentials"."aws"."cloud-credentials"."fake-credential-name"."details"."access-key"' | check "fake-access-key"

	juju remove-credential aws fake-credential-name --client

	juju credentials aws --format=json | jq -r '."client-credentials"."aws"."cloud-credentials"."fake-credential-name"."details"."access-key"' | check null

	destroy_model "${model_name}"
}

test_add_remove_credential() {
	if [ "$(skip 'test_add_remove_credential')" ]; then
		echo "==> TEST SKIPPED: add remove credential"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_add_remove_credential"
	)
}
