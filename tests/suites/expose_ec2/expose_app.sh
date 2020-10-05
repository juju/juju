run_expose_app_ec2() {
    echo

    file="${TEST_DIR}/test-expose-app-ec2.log"

    ensure "expose-app" "${file}"

    # Deploy test charm
    juju deploy cs:~jameinel/ubuntu-lite-7
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

    juju run --unit ubuntu-lite/0 "open-port 1337-1339/tcp"
    juju run --unit ubuntu-lite/0 "open-port 1234/tcp --endpoints ubuntu"

    # Test the backwards-compatible version of opened-ports where the output
    # includes the unique set of opened ports for all endpoints.
    # Note that 'juju run' injects a trailing line-feed tot he command output
    # so we need to use echo to generate our expectation string.
    exp=$(echo "1234/tcp 1337-1339/tcp" | tr '\n' ' ')
    got=$(juju run --unit ubuntu-lite/0 "opened-ports" | tr '\n' ' ')
    if [ "$got" != "$exp" ]; then
      # shellcheck disable=SC2046
      echo $(red "expected opened-ports output to be:\n${exp}\nGOT:\n${got}")
      exit 1
    fi

    # Try the new version where we group by endpoint.
    # Note that 'juju run' injects a trailing line-feed tot he command output
    # so we need to use echo to generate our expectation string.
    exp=$(echo "1234/tcp (ubuntu) 1337-1339/tcp (*)" | tr '\n' ' ')
    got=$(juju run --unit ubuntu-lite/0 "opened-ports --endpoints" | tr '\n' ' ')
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
    sleep 2 # wait for firewall worker to detect and apply the changes

    # Range 1337-1339 is opened for all endpoints. We expect it to be reachable
    # by the expose-all CIDR list plus the CIDR for the ubuntu endpoint.
    assert_ipv4_ingress_cidrs_for_port_range "1337" "1339" "10.0.0.0/24,10.42.0.0/16,192.168.0.0/24"

    # Port 1234 should only be opened for the CIDR specified for the ubuntu endpoint
    assert_ipv4_ingress_cidrs_for_port_range "1234" "1234" "10.42.0.0/16"
    assert_ipv6_ingress_cidrs_for_port_range "1234" "1234" "2002:0:0:1234::/64"
}

# assert_ipv4_ingress_cidrs_for_port_range $from_port, $to_port $exp_cidrs
assert_ipv4_ingress_cidrs_for_port_range() {
  assert_ingress_cidrs_for_port_range "$1" "$2" "$3" "ipv4"
}

# assert_ipv6_ingress_cidrs_for_port_range $from_port, $to_port $exp_cidrs
assert_ipv6_ingress_cidrs_for_port_range() {
  assert_ingress_cidrs_for_port_range "$1" "$2" "$3" "ipv6"
}

assert_ingress_cidrs_for_port_range() {
    local from_port to_port exp_cidrs cidr_type

    from_port=${1}
    to_port=${2}
    exp_cidrs=${3}
    cidr_type=${4}

    # shellcheck disable=SC2086
    secgrp_list=$(aws ec2 describe-security-groups --filters Name=ip-permission.from-port,Values=${from_port} Name=ip-permission.to-port,Values=${to_port})
    if [ "$cidr_type" = "ipv4" ]; then
      # shellcheck disable=SC2086
      got_cidrs=$(echo ${secgrp_list} | jq -r ".SecurityGroups[0].IpPermissions | .[] | select(.FromPort == ${from_port} and .ToPort == ${to_port}) | .IpRanges | .[] | .CidrIp" | sort | paste -sd, -)
    else
      # shellcheck disable=SC2086
      got_cidrs=$(echo ${secgrp_list} | jq -r ".SecurityGroups[0].IpPermissions | .[] | select(.FromPort == ${from_port} and .ToPort == ${to_port}) | .Ipv6Ranges | .[] | .CidrIpv6" | sort | paste -sd, -)
    fi

    if [ "$got_cidrs" != "$exp_cidrs" ]; then
      # shellcheck disable=SC2046
      echo $(red "expected generated EC2 ${cidr_type} ingress CIDRs for range [${from_port}, ${to_port}] to be:\n${exp_cidrs}\nGOT:\n${got_cidrs}")
      exit 1
    fi
}

assert_export_bundle_output_includes_exposed_endpoints() {
    echo "==> Checking that export-bundle output contains the exposed endpoint settings"

    got=$(juju export-bundle)
    exp=$(cat <<-EOF
series: bionic
applications:
  ubuntu-lite:
    charm: cs:~jameinel/ubuntu-lite-7
    num_units: 1
    to:
    - "0"
machines:
  "0": {}
--- # overlay.yaml
applications:
  ubuntu-lite:
    exposed-endpoints:
      "":
        expose-to-cidrs:
        - 10.0.0.0/24
        - 192.168.0.0/24
      ubuntu:
        expose-to-cidrs:
        - 10.42.0.0/16
        - 2002:0:0:1234::/64
EOF
)

    if [ "$got" != "$exp" ]; then
      # shellcheck disable=SC2046
      echo $(red "expected exported bundle to be:\n${exp}\nGOT:\n${got}")
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
