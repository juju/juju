# Copyright 2024 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.

run_deploy_default_series() {
	echo

	model_name="test-deploy-default-base"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju model-config default-base=ubuntu@22.04
	juju deploy ubuntu --storage "files=tmpfs"
	wait_for "ubuntu" "$(idle_condition "ubuntu")"

	ubuntu_base_name=$(juju status --format=json | jq ".applications.ubuntu.base.name")
	ubuntu_base_ch=$(juju status --format=json | jq ".applications.ubuntu.base.channel")
	echo "$ubuntu_base_name" | check "ubuntu"
	echo "$ubuntu_base_ch" | check "22.04"

	destroy_model "${model_name}"
}

run_deploy_not_default_series() {
	echo

	model_name="test-deploy-not-default-base"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju model-config default-base=ubuntu@20.04
	juju deploy ubuntu --storage "files=tmpfs" --base ubuntu@24.04
	wait_for "ubuntu" "$(idle_condition "ubuntu")"

	ubuntu_base_name=$(juju status --format=json | jq ".applications.ubuntu.base.name")
	ubuntu_base_ch=$(juju status --format=json | jq ".applications.ubuntu.base.channel")
	echo "$ubuntu_base_name" | check "ubuntu"
	echo "$ubuntu_base_ch" | check "24.04"

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
