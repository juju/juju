run_show_clouds() {
	echo

	mkdir -p "${TEST_DIR}/juju"
	echo "" >>"${TEST_DIR}/juju/public-clouds.yaml"
	echo "" >>"${TEST_DIR}/juju/credentials.yaml"

	OUT=$(XDG_DATA_HOME="${TEST_DIR}" juju clouds --local --format=json 2>/dev/null | jq '.[] | select(.defined != "built-in")')
	if [ -n "${OUT}" ]; then
		echo "expected empty, got ${OUT}"
		exit 1
	fi

	cp ./tests/suites/cli/clouds/public-clouds.yaml "${TEST_DIR}"/juju/public-clouds.yaml
	OUT=$(XDG_DATA_HOME="${TEST_DIR}" juju clouds --local --format=json 2>/dev/null | jq '.[] | select(.defined != "built-in")')
	if [ -n "${OUT}" ]; then
		echo "expected empty, got ${OUT}"
		exit 1
	fi

	EXPECTED=$(
		cat <<'EOF'
{
  "defined": "public",
  "type": "ec2",
  "auth-types": [
    "userpass"
  ],
  "regions": {
    "us-east-1": {
      "endpoint": "https://ec2.us-east-1.amazonaws.com"
    },
    "us-east-2": {
      "endpoint": "https://ec2.us-east-2.amazonaws.com"
    }
  }
}
EOF
	)

	OUT=$(XDG_DATA_HOME="${TEST_DIR}" juju clouds --all --format=json 2>/dev/null | jq '.[] | select(.defined != "built-in")')
	if [ "${OUT}" != "${EXPECTED}" ]; then
		echo "expected ${EXPECTED}, got ${OUT}"
		exit 1
	fi
}

test_display_clouds() {
	if [ "$(skip 'test_display_clouds')" ]; then
		echo "==> TEST SKIPPED: display clouds"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_show_clouds"
	)
}
