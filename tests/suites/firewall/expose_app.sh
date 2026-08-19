run_expose_app_ec2() {
	echo

	file="${TEST_DIR}/test-expose-app-ec2.log"

	ensure "expose-app" "${file}"

	# Deploy test charm
	juju deploy ubuntu-lite
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"

	# Open ports and verify hook tool behavior
	assert_opened_ports_output "ubuntu-lite" "1337-1339/tcp" "ubuntu" "1234/tcp"

	# Ensure that CIDRs are correctly generated
	assert_ingress_cidrs_for_exposed_app

	destroy_model "expose-app"
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
