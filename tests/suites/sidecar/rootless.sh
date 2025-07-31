run_non_root_charm() {
	echo

	file="${TEST_DIR}/test-rootless-non-root-charm.log"

	ensure "test-rootless-non-root-charm" "${file}"

	# shellcheck disable=SC2046
	juju deploy $(pack_charm ./testcharms/charms/sidecar-non-root) --resource ubuntu=public.ecr.aws/ubuntu/ubuntu:22.04

	wait_for "sidecar-non-root" "$(idle_condition "sidecar-non-root" 0)"
	sleep 10 # wait for logs

	output=$(juju debug-log --replay)
	check_contains "$output" "charm=170"
	check_contains "$output" "sudo=no"
	check_contains "$output" "rootless=10000"
	check_contains "$output" "rootful=0"

	destroy_model "test-rootless-non-root-charm"
}

run_sudoer_charm() {
	echo

	file="${TEST_DIR}/test-rootless-sudoer-charm.log"

	ensure "test-rootless-sudoer-charm" "${file}"

	# shellcheck disable=SC2046
	juju deploy $(pack_charm ./testcharms/charms/sidecar-sudoer) --resource ubuntu=public.ecr.aws/ubuntu/ubuntu:22.04

	wait_for "sidecar-sudoer" "$(idle_condition "sidecar-sudoer" 0)"
	sleep 10 # wait for logs

	output=$(juju debug-log --replay)
	check_contains "$output" "charm=171"
	check_contains "$output" "sudo=yes"
	check_contains "$output" "rootless=10000"
	check_contains "$output" "rootful=0"

	destroy_model "test-rootless-sudoer-charm"
}

test_rootless() {
	if [ "$(skip 'test_rootless')" ]; then
		echo "==> TEST SKIPPED: test_rootless"
		return
	fi

	(
		set_verbosity

		echo "==> Checking for dependencies"
		check_dependencies charmcraft

		cd .. || exit

		run "run_non_root_charm"
		run "run_sudoer_charm"
	)
}
