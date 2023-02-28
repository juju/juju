run_charmhub_info() {
	echo
	output=$(juju info ubuntu 2>&1 || true)
	#
	# These keys will not be printed if the data does not exist.
	# Check the data is there.
	#
	check_contains "$output" "supports"
	check_contains "$output" "name"
	check_contains "$output" "summary"
	check_contains "$output" "channels"

	#
	# Only available via flag, which is not used here.
	#
	check_not_contains "$output" "config"
}

run_charmhub_info_config() {
	echo
	output=$(juju info ubuntu --config 2>&1 || true)
	#
	# These keys will not be printed if the data does not exist.
	# Check the data is there.
	#
	check_contains "$output" "supports"
	check_contains "$output" "name"
	check_contains "$output" "summary"
	check_contains "$output" "channels"

	#
	# Only printed with the flag.
	#
	check_contains "$output" "config"
}

run_charmhub_info_json() {
	echo
	output=$(juju info ubuntu --format json | jq .charm.config.Options.hostname.Type)
	check_contains "${output}" "string"
}

run_charmstore_info() {
	echo
	output=$(juju info cs:ubuntu 2>&1 || true)
	check_contains "$output" 'ERROR charm or bundle name, "cs:ubuntu", is not valid'
}

test_charmhub_info() {
	if [ "$(skip 'test_charmhub_info')" ]; then
		echo "==> TEST SKIPPED: Charm Hub info"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_charmhub_info"
		run "run_charmhub_info_config"
		run "run_charmhub_info_json"
		run "run_charmstore_info"
	)
}
