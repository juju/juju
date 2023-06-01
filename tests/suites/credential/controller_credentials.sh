run_controller_credentials() {
	echo

	juju show-cloud --controller "${BOOTSTRAPPED_JUJU_CTRL_NAME}" aws 2>/dev/null || juju add-cloud --controller "${BOOTSTRAPPED_JUJU_CTRL_NAME}" aws --force
	juju add-credential aws -f "./tests/suites/credential/credentials-data/fake-credentials.yaml" --controller "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	OUT=$(juju credentials --controller "${BOOTSTRAPPED_JUJU_CTRL_NAME}" --format=json 2>/dev/null | jq '.[].aws."cloud-credentials"')

	EXPECTED=$(
		cat <<'EOF'
  "fake-credential-name": {
    "auth-type": "access-key",
    "details": {
      "access-key": "fake-access-key"
    }
  }
EOF
	)
	# Controller has more than one credential, just check the one we added.
	if [[ ${OUT} != *"${EXPECTED}"* ]]; then
		echo "expected ${EXPECTED}, got ${OUT}"
		exit 1
	fi

	OUT=$(juju credentials --controller ${BOOTSTRAPPED_JUJU_CTRL_NAME} --show-secrets --format=json 2>/dev/null | jq '.[].aws."cloud-credentials"')
	EXPECTED=$(
		cat <<'EOF'
  "fake-credential-name": {
    "auth-type": "access-key",
    "details": {
      "access-key": "fake-access-key",
      "secret-key": "fake-secret-key"
    }
  }

EOF
	)
	# Controller has more than one credential, just check the one we added.
	if [[ ${OUT} != *"${EXPECTED}"* ]]; then
		echo "expected ${EXPECTED}, got ${OUT}"
		exit 1
	fi
}

test_controller_credentials() {
	if [ "$(skip 'test_credentials')" ]; then
		echo "==> TEST SKIPPED: credentials"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		file="${TEST_DIR}/test-credential.log"
		bootstrap "test-credential" "${file}"

		run "run_controller_credentials"
		# TODO: remove credential afterwards - even if test fails
		# juju remove-credential aws fake-credential-name || true

		destroy_controller "test-credential"
	)
}
