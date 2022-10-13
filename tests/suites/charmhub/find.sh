run_charmhub_find_specific() {
	echo
	output=$(juju find ubuntu 2>&1 || true)
	check_not_contains "${output}" "No matching charms for"
	check_contains "${output}" ubuntu
}

run_charmhub_find_all() {
	echo
	output=$(juju find 2>&1 || true)

	check_contains "${output}" "No search term specified. Here are some interesting charms"
}

run_charmhub_find_json() {
	echo
	# There should always be 1 charm with ubuntu in the name,
	# charms should always have at least 1 supported base.
	output=$(juju find ubuntu --format json | jq '.[0].supports | length')
	check_gt "${output}" "0"
}

run_charmhub_find_not_matching() {
	echo
	output=$(juju find "nosuchcharmorbundleeverl33t" 2>&1)
	check_contains "${output}" "No matching charms or bundles"
}

run_charmstore_find() {
	echo
	output=$(juju find cs:ubuntu 2>&1 || true)
	check_contains "${output}" "No matching charms or bundles"
}

test_charmhub_find() {
	if [ "$(skip 'test_charmhub_find')" ]; then
		echo "==> TEST SKIPPED: Charm Hub find"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_charmhub_find_specific"
		run "run_charmhub_find_all"
		run "run_charmhub_find_json"
		run "run_charmhub_find_not_matching"
		run "run_charmstore_find"
	)
}
