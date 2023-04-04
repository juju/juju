run_unregister() {
	echo

	echo "Create temporary controllers.yaml file"
	mkdir -p "${TEST_DIR}/juju"
	echo "" >>"${TEST_DIR}/juju/controllers.yaml"

	CONTROLLERS=$(
		cat <<'EOF'
controllers:
    unregister-test:
        uuid: c2a22492-e551-4856-850c-f3df36e9a828
        api-endpoints: ['10.209.4.183:17070']
        ca-cert: |
            -----BEGIN CERTIFICATE-----
            XXX==
            -----END CERTIFICATE-----
        cloud: lxd
        region: default
        type: lxd
        agent-version: 2.9.35.1
        controller-machine-count: 1
        active-controller-machine-count: 0
        machine-count: 1
EOF
	)

	echo "Set fake data about unregister-test controller in controllers.yaml"
	echo "${CONTROLLERS}" >>"${TEST_DIR}/juju/controllers.yaml"

	echo "Unregister the unregister-test controller"
	JUJU_DATA="${TEST_DIR}/juju" juju unregister --no-prompt "unregister-test"

	echo "Check that temporary controllers.yaml has no controllers' data"
	EXPECTED="controllers: {}"
	OUT=$(cat "${TEST_DIR}/juju/controllers.yaml")
	if [ "${OUT}" != "${EXPECTED}" ]; then
		echo "expected ${EXPECTED}, got ${OUT}"
		exit 1
	fi

	echo "Check 'juju switch' returns the error"
	EXPECTED='ERROR "unregister-test" is not the name of a model or controller'
	if ! OUT=$(JUJU_DATA="${TEST_DIR}/juju" juju switch "unregister-test" 2>&1); then
		if [ "${OUT}" != "${EXPECTED}" ]; then
			echo "expected ${EXPECTED}, got ${OUT}"
			exit 1
		fi
		echo "'juju switch' returned the error as expected"
		exit 0
	fi
}

test_unregister() {
	if [ -n "$(skip 'test_unregister')" ]; then
		echo "==> SKIP: Asked to skip controller unregister tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_unregister"
	)
}
