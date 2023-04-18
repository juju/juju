run_deploy_default_series() {
	echo

	model_name="test-deploy-default-series"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju model-config default-series=bionic
	juju deploy ubuntu --storage "files=tmpfs"
	wait_for "ubuntu" "$(idle_condition "ubuntu")"
	series=$(juju status --format=json | jq ".applications.ubuntu.series")
	echo "$series" | check "bionic"

	destroy_model "${model_name}"
}

run_deploy_default_series_cs() {
	echo

	model_name="test-deploy-default-series-cs"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju model-config default-series=bionic
	juju deploy cs:ubuntu --storage "files=tmpfs"
	wait_for "ubuntu" "$(idle_condition "ubuntu")"
	series=$(juju status --format=json | jq ".applications.ubuntu.series")
	echo "$series" | check "bionic"

	destroy_model "${model_name}"
}

run_deploy_not_default_series() {
	echo

	model_name="test-deploy-not-default-series"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju model-config default-series=bionic
	juju deploy ubuntu --storage "files=tmpfs" --series focal
	wait_for "ubuntu" "$(idle_condition "ubuntu")"
	series=$(juju status --format=json | jq ".applications.ubuntu.series")
	echo "$series" | check "focal"

	destroy_model "${model_name}"
}

run_deploy_not_default_series_cs() {
	echo

	model_name="test-deploy-not-default-series-cs"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju model-config default-series=bionic
	juju deploy cs:ubuntu --storage "files=tmpfs" --series focal
	wait_for "ubuntu" "$(idle_condition "ubuntu")"
	series=$(juju status --format=json | jq ".applications.ubuntu.series")
	echo "$series" | check "focal"

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
		run "run_deploy_default_series_cs"
		run "run_deploy_not_default_series"
		run "run_deploy_not_default_series_cs"
	)
}
