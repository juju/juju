# Bootstraps alternate controller in test
bootstrap_alt_controller() {
	local name

	name=${1}

	START_TIME=$(date +%s)
	echo "====> Bootstrapping ${name}"

	# Unset to re-generate from the new agent-version.
	unset BOOTSTRAP_ADDITIONAL_ARGS

	file="${TEST_DIR}/${name}.log"
	juju_bootstrap "${BOOTSTRAP_CLOUD}" "${name}" "misc" "${file}"

	END_TIME=$(date +%s)
	echo "====> Bootstrapped ${name} ($((END_TIME - START_TIME))s)"
}
