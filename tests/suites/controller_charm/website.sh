run_website() {
	echo

	file="${TEST_DIR}/test-website.log"

	ensure "test-website" "${file}"

	juju switch controller
	juju offer controller:website website
	juju switch test-website
	juju deploy haproxy
	juju relate haproxy controller.website
	wait_for "haproxy" "$(idle_condition "haproxy")"

	# Ensure controller charm still "active", not "error"
	local controller_charm_status
	controller_charm_status=$(juju status -m controller --format json | jq -r '.applications.controller."application-status".current')
	if [[ $controller_charm_status != 'active' ]]; then
		exit 1
	fi

	destroy_model "test-website"
}

test_website() {
	if [ "$(skip 'test_website')" ]; then
		echo "==> TEST SKIPPED: website relation"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_website"
	)
}
