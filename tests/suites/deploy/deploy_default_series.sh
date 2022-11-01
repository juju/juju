run_deploy_default_series() {
	echo

	model_name="test-deploy-default-series"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju model-config default-series=jammy
	juju deploy ubuntu
	juju deploy cs:ubuntu csubuntu

  juju status --format=json | jq .
	ubuntu_series=$(juju status --format=json | jq ".applications.ubuntu.series")
	echo "$ubuntu_series" | check "jammy"

	csubuntu_series=$(juju status --format=json | jq ".applications.csubuntu.series")
	echo "$csubuntu_series" | check "jammy"

	destroy_model "${model_name}"
}

run_deploy_not_default_series() {
	echo

	model_name="test-deploy-not-default-series"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju model-config default-series=bionic
	juju deploy ubuntu --series focal
	juju deploy cs:ubuntu csubuntu --series focal

	ubuntu_series=$(juju status --format=json | jq ".applications.ubuntu.series")
	echo "$ubuntu_series" | check "focal"

	csubuntu_series=$(juju status --format=json | jq ".applications.csubuntu.series")
	echo "$csubuntu_series" | check "focal"

	destroy_model "${model_name}"
}

test_deploy_default_series() {
	if [ "$(skip 'test_deploy_default_series')" ]; then
		echo "==> TEST SKIPPED: deploy default series"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_deploy_default_series"
		run "run_deploy_not_default_series"
	)
}
