# Adds and removes credentials from file to our juju client.
run_add_remove_credential() {
	# Echo out to ensure nice output to the test suite.
	echo

	echo "Add fake credential"
	JUJU_DATA="${TEST_DIR}/juju" juju add-credential aws -f ./tests/suites/credential/credentials-data/fake-credentials.yaml --client

	echo "Check fake credential"
	JUJU_DATA="${TEST_DIR}/juju" juju credentials aws --format=json | jq -r '."client-credentials"."aws"."cloud-credentials"."fake-credential-name"."details"."access-key"' | check "fake-access-key"

	echo "Remove fake credential"
	JUJU_DATA="${TEST_DIR}/juju" juju remove-credential aws fake-credential-name --client

	echo "Check fake credential is deleted"
	JUJU_DATA="${TEST_DIR}/juju" juju credentials aws --format=json | jq -r '."client-credentials"."aws"."cloud-credentials"."fake-credential-name"."details"."access-key"' | check null

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
