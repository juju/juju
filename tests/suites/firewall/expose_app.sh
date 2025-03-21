run_expose_app_ec2() {
	echo

	file="${TEST_DIR}/test-expose-app-ec2.log"

	ensure "expose-app" "${file}"

	# Deploy test charm
	juju deploy ubuntu-lite
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"

	# Open ports and verify hook tool behavior
	assert_opened_ports_output

	# Ensure that CIDRs are correctly generated
	assert_ingress_cidrs_for_exposed_app

	# Ensure that the per-endpoint rules are included in exported bundles
	assert_export_bundle_output_includes_exposed_endpoints

	destroy_model "expose-app"
}

assert_opened_ports_output() {
	echo "==> Checking open/opened-ports hook tools work as expected"

	juju exec --unit ubuntu-lite/0 "open-port 1337-1339/tcp"
	juju exec --unit ubuntu-lite/0 "open-port 1234/tcp --endpoints ubuntu"

	# Test the backwards-compatible version of opened-ports where the output
	# includes the unique set of opened ports for all endpoints.
	exp="1234/tcp 1337-1339/tcp"
	got=$(juju exec --unit ubuntu-lite/0 "opened-ports" | tr '\n' ' ' | sed -e 's/[[:space:]]*$//')
	if [ "$got" != "$exp" ]; then
		# shellcheck disable=SC2046
		echo $(red "expected opened-ports output to be:\n${exp}\nGOT:\n${got}")
		exit 1
	fi

	# Try the new version where we group by endpoint.
	exp="1234/tcp (ubuntu) 1337-1339/tcp (*)"
	got=$(juju exec --unit ubuntu-lite/0 "opened-ports --endpoints" | tr '\n' ' ' | sed -e 's/[[:space:]]*$//')
	if [ "$got" != "$exp" ]; then
		# shellcheck disable=SC2046
		echo $(red "expected opened-ports output when using --endpoints to be:\n${exp}\nGOT:\n${got}")
		exit 1
	fi
}

assert_ingress_cidrs_for_exposed_app() {
	echo "==> Checking that expose --to-cidrs works as expected"

	juju expose ubuntu-lite --to-cidrs 10.0.0.0/24,192.168.0.0/24
	juju expose ubuntu-lite --endpoints ubuntu # expose to the world
	# overwrite previous command
	juju expose ubuntu-lite --endpoints ubuntu --to-cidrs 10.42.0.0/16,2002:0:0:1234::/64

	echo "==> Waiting for the security group rules will be updated"
	# Range 1337-1339 is opened for all endpoints. We expect it to be reachable
	# by the expose-all CIDR list plus the CIDR for the ubuntu endpoint.
	wait_for_aws_ingress_cidrs_for_port_range "1337" "1339" "10.0.0.0/24,10.42.0.0/16,192.168.0.0/24" "ipv4"

	# Port 1234 should only be opened for the CIDR specified for the ubuntu endpoint
	wait_for_aws_ingress_cidrs_for_port_range "1234" "1234" "10.42.0.0/16" "ipv4"
	wait_for_aws_ingress_cidrs_for_port_range "1234" "1234" "2002:0:0:1234::/64" "ipv6"
}

assert_export_bundle_output_includes_exposed_endpoints() {

	echo "==> Checking that export-bundle output contains the exposed endpoint settings"

# TODO(gfouillet) - recover from 3.6, delete whenever export bundle is restored or deleted
    got=$(juju export-bundle 2>&1 1>/dev/null)
    if [[ "$got" != *"not implemented"* ]]; then
        echo "ERROR: export-bundle should return 'not implemented'."
        exit 1
    fi
}

test_expose_app_ec2() {
	if [ "$(skip 'test_expose_app_ec2')" ]; then
		echo "==> TEST SKIPPED: juju expose_app_ec2"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_expose_app_ec2" "$@"
	)
}
