run_show_clouds() {
	echo

	mkdir -p "${TEST_DIR}/juju"
	echo "" >>"${TEST_DIR}/juju/public-clouds.yaml"
	echo "" >>"${TEST_DIR}/juju/credentials.yaml"

	OUT=$(JUJU_DATA="${TEST_DIR}/juju" juju clouds --client --format=json | jq '.[] | select(.defined != "built-in")')
	if [ -n "${OUT}" ]; then
		echo "expected empty, got ${OUT}"
		exit 1
	fi

	cp ./tests/suites/cli/clouds/public-clouds.yaml "${TEST_DIR}"/juju/public-clouds.yaml
	OUT=$(JUJU_DATA="${TEST_DIR}/juju" juju clouds --client --format=json | jq '.[] | select(.defined != "built-in")')
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
    "access-key"
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

	OUT=$(JUJU_DATA="${TEST_DIR}/juju" juju clouds --all --format=json | jq '.[] | select(.defined != "built-in")')
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

	CLOUD_LIST=$(JUJU_DATA="${TEST_DIR}/juju" juju clouds --client --format=json | jq 'with_entries(select(.value.defined != "built-in"))')
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
     - access-key
     endpoint: 10.247.0.3
     regions:
       QA:
         endpoint: 10.247.0.3
     type: vsphere
EOF
	)

	echo "${CLOUDS}" >>"${TEST_DIR}/juju/clouds.yaml"
	CLOUD_LIST=$(JUJU_DATA="${TEST_DIR}/juju" juju clouds --client --format=json | jq -S 'with_entries(select(
	                                                  .value.defined != "built-in")) | with_entries((select(.value.defined == "local")
	                                                  | del(.value.defined) |  del(.value.description)))')
	EXPECTED=$(echo "${CLOUDS}" | yq -S '.[] | del(.clouds) | .[] |= ({endpoint} as $endpoint | .[] |= walk(
                                                  (objects | select(contains($endpoint))) |= del(.endpoint)
                                                ))')
	if [ "${CLOUD_LIST}" != "${EXPECTED}" ]; then
		echo "expected ${EXPECTED}, got ${CLOUD_LIST}"
		exit 1
	fi

	CLOUD_LIST=$(JUJU_DATA="${TEST_DIR}/juju" juju show-cloud finfolk-vmaas --format json --client | jq -S '.[] | with_entries((select(.value!= null)))')
	EXPECTED=$(
		cat <<'EOF' | jq -S
	{
    "auth-types": [
      "oauth1"
    ],
    "defined": "local",
    "description": "Metal As A Service",
    "endpoint": "http://10.125.0.10:5240/MAAS/",
    "name": "finfolk-vmaas",
    "summary": "Client cloud \"finfolk-vmaas\"",
    "type": "maas"
  }
EOF
	)

	if [ "${CLOUD_LIST}" != "${EXPECTED}" ]; then
		echo "expected ${EXPECTED}, got ${CLOUD_LIST}"
		exit 1
	fi
}

run_controller_clouds() {
	echo

	juju add-cloud my-ec2 -f "./tests/suites/cli/clouds/myclouds.yaml" --force --controller ${BOOTSTRAPPED_JUJU_CTRL_NAME}

	## starting from Juju 4 we use user UUID instead of user name, not to overheat the test, we will just replace
	## the user UUID with "admin-uuid" value in the output to make the test output easier to compare.
	OUT=$(juju clouds --controller ${BOOTSTRAPPED_JUJU_CTRL_NAME} --format=json | jq '."my-ec2" | .users |= with_entries(if .key != "admin-uuid" then .key = "admin-uuid" else . end)')

	EXPECTED=$(
		cat <<'EOF'
{
  "defined": "public",
  "type": "ec2",
  "auth-types": [
    "access-key"
  ],
  "regions": {
    "us-west-1": {
      "endpoint": "https://ec2.us-west-1.amazonaws.com"
    },
    "us-west-2": {
      "endpoint": "https://ec2.us-west-2.amazonaws.com"
    }
  },
  "users": {
    "admin-uuid": {
      "display-name": "admin",
      "access": "admin"
    }
  }
}
EOF
	)
	# Controller has more than one cloud, just check the one we added.
	if [[ ${OUT} != "${EXPECTED}" ]]; then
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
		run "run_assess_clouds"
		run "run_controller_clouds"
	)
}
