run_show_clouds() {
	echo

	mkdir -p "${TEST_DIR}/juju"
	echo "" >>"${TEST_DIR}/juju/public-clouds.yaml"
	echo "" >>"${TEST_DIR}/juju/credentials.yaml"

	OUT=$(JUJU_DATA="${TEST_DIR}/juju" juju clouds --client --format=json 2>/dev/null | jq '.[] | select(.defined != "built-in")')
	if [ -n "${OUT}" ]; then
		echo "expected empty, got ${OUT}"
		exit 1
	fi

	cp ./tests/suites/cli/clouds/public-clouds.yaml "${TEST_DIR}"/juju/public-clouds.yaml
	OUT=$(JUJU_DATA="${TEST_DIR}/juju" juju clouds --client --format=json 2>/dev/null | jq '.[] | select(.defined != "built-in")')
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

	OUT=$(JUJU_DATA="${TEST_DIR}/juju" juju clouds --all --format=json 2>/dev/null | jq '.[] | select(.defined != "built-in")')
	if [ "${OUT}" != "${EXPECTED}" ]; then
		echo "expected ${EXPECTED}, got ${OUT}"
		exit 1
	fi
}

run_assess_clouds() {
	echo

	mkdir -p "${TEST_DIR}/juju"
	echo "" >>"${TEST_DIR}/juju/public-clouds.yaml"
	echo "" >>"${TEST_DIR}/juju/credentials.yaml"

	CLOUD_LIST=$(JUJU_DATA="${TEST_DIR}/juju" juju clouds --client --format=json 2>/dev/null | jq 'with_entries(select(.value.defined != "built-in"))')
	EXPECTED={}
	if [ "${CLOUD_LIST}" != "${EXPECTED}" ]; then
		echo "expected ${EXPECTED}, got ${CLOUD_LIST}"
		exit 1
	fi

	CLOUDS=$(
		cat <<'EOF'
 clouds:
   finfolk-vmaas:
     auth-types:
     - oauth1
     endpoint: http://10.125.0.10:5240/MAAS/
     type: maas
   vsphere:
     auth-types:
     - userpass
     endpoint: 10.247.0.3
     regions:
       QA:
         endpoint: 10.247.0.3
     type: vsphere
EOF
	)

	echo "${CLOUDS}" >>"${TEST_DIR}/juju/clouds.yaml"
	CLOUD_LIST=$(JUJU_DATA="${TEST_DIR}/juju" juju clouds --client --format=json 2>/dev/null | jq -S 'with_entries(select(
	                                                  .value.defined != "built-in")) | with_entries((select(.value.defined == "local")
	                                                  | del(.value.defined) |  del(.value.description)))')
	EXPECTED=$(echo "${CLOUDS}" | yq -I0 -oj | jq -S '.[] | del(.clouds) | .[] |= ({endpoint} as $endpoint | .[] |= walk(
                                                  (objects | select(contains($endpoint))) |= del(.endpoint)
                                                ))')
	if [ "${CLOUD_LIST}" != "${EXPECTED}" ]; then
		echo "expected ${EXPECTED}, got ${CLOUD_LIST}"
		exit 1
	fi

	CLOUD_LIST=$(JUJU_DATA="${TEST_DIR}/juju" juju show-cloud finfolk-vmaas --format yaml --client 2>/dev/null | yq -I0 -oj | jq -S 'with_entries((select(.value!= null)))')
	EXPECTED=$(
		cat <<'EOF' | jq -S
	{
    "auth-types": [
      "oauth1"
    ],
    "defined": "local",
    "description": "Metal As A Service",
    "endpoint": "http://10.125.0.10:5240/MAAS/",
    "type": "maas"
  }
EOF
	)

	if [ "${CLOUD_LIST}" != "${EXPECTED}" ]; then
		echo "expected ${EXPECTED}, got ${CLOUD_LIST}"
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
		run "run_assess_clouds"
	)
}
