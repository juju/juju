run_model_config_isomorphic() {
	echo

	file="${TEST_DIR}/model-config-isomorphic.log"
	ensure "model-config-isomorphic" "${file}"

	FILE=$(mktemp)

	juju model-config --format=yaml | juju model-config --ignore-read-only-fields --file -

	destroy_model "model-config-isomorphic"
}

run_model_config_cloudinit_userdata() {
	echo

	file="${TEST_DIR}/model-config-cloudinit-userdata.log"
	ensure "model-config-cloudinit-userdata" "${file}"

	FILE=$(mktemp)

	cat <<EOF >"${FILE}"
cloudinit-userdata: |
  packages:
    - jq
    - shellcheck
EOF

	juju model-config --file "${FILE}"
	OUT=$(juju model-config cloudinit-userdata)
	echo "${OUT}" | grep -q "shellcheck"

	# cloudinit-userdata is not present from the default tabular output
	! juju model-config cloudinit-userdata | grep -q "^cloudinit-userdata: |$"

	# cloudinit-userdata is hidden in the normal output
	OUT=$(juju model-config)
	echo "${OUT}" | grep -q "<value set, see juju model-config cloudinit-userdata>"

	destroy_model "model-config-cloudinit-userdata"
}

test_model_config() {
	if [ "$(skip 'test_model_config')" ]; then
		echo "==> TEST SKIPPED: model config"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		# TODO(anvial): this subtest is commented because we support both 'default-series' and 'default-base'.
		# We need to return this subtest back when we fully drop series support.
		#	run "run_model_config_isomorphic"
		run "run_model_config_cloudinit_userdata"
	)
}
