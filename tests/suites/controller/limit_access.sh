verify_model_network_tag() {
	local network_tag="$1" sourceRange="$2"

	case "${BOOTSTRAP_PROVIDER:-}" in
	"ec2")
		sg="$(aws ec2 describe-security-groups --filters Name=group-name,Values="$network_tag")"
		jq -r ".SecurityGroups[] |.IpPermissions[] | select(.FromPort == 22) | .IpRanges[0].CidrIp" <<<"${sg}" | check "${sourceRange}"
		;;
	"gce")
		# Ensure only one allowed item, which is ssh port
		default_rule=$(gcloud compute firewall-rules list \
			--filter="targetTags.list():${network_tag}" \
			--format=json)
		echo "${default_rule}" | jq -r '.[0].allowed[0].ports | length' | check "1"
		echo "${default_rule}" | jq -r '.[0].allowed[0].ports[0]' | check "22"
		echo "${default_rule}" | jq -r '.[0].sourceRanges[0]' | check "${sourceRange}"
		;;
	*)
		echo "Aborting, we shouldn't be here"
		exit 1
		;;
	esac
}

verify_instance_network_tag() {
	local network_tag="$1" sourceRange="$2"

	# Ensure two items, one is api port, another for ssh server port
	case "${BOOTSTRAP_PROVIDER:-}" in
	"ec2")
		sg="$(aws ec2 describe-security-groups --filters Name=group-name,Values="$network_tag")"
		jq -r ".SecurityGroups[] |.IpPermissions[] | select(.FromPort == 17070) | .IpRanges[0].CidrIp" <<<"${sg}" | check "${sourceRange}"
		jq -r ".SecurityGroups[] |.IpPermissions[] | select(.FromPort == 17022) | .IpRanges[0].CidrIp" <<<"${sg}" | check "${sourceRange}"
		;;
	"gce")
		default_rule=$(gcloud compute firewall-rules list \
			--filter="targetTags.list():${network_tag}" \
			--format=json)
		echo "${default_rule}" | jq -r '.[0].allowed[0].ports | length' | check "2"
		echo "${default_rule}" | jq -r '.[0].allowed[0].ports[0]' | check "17022"
		echo "${default_rule}" | jq -r '.[0].allowed[0].ports[1]' | check "17070"
		echo "${default_rule}" | jq -r '.[0].sourceRanges[0]' | check "${sourceRange}"
		;;
	*)
		echo "Aborting, we shouldn't be here"
		exit 1
		;;
	esac
}

run_limit_access() {
	echo

	file="${TEST_DIR}/limit_access.log"

	ensure "limit-access" "${file}"

	juju add-machine
	wait_for_machine_agent_status "0" "started"

	machine_info="$(juju list-machines -m controller --format=json)"
	instance_id="$(jq -r '.machines["0"]."instance-id"' <<<"$machine_info")"
	region_or_az=$(region_or_availability_zone)
	model_uuid=$(juju show-model controller --format json | jq -r '.["controller"]["model-uuid"]')
	model_network_tag="juju-${model_uuid}"
	verify_model_network_tag "${model_network_tag}" "0.0.0.0/0"

	network_tag_or_group=$(instance_network_tag_or_group)
	verify_instance_network_tag "${network_tag_or_group}" "0.0.0.0/0"

	echo "Limit access to controller, which only affects controller instance firewall rule"
	juju expose -m controller controller --to-cidrs 10.0.0.0/24
	verify_model_network_tag "${model_network_tag}" "0.0.0.0/0"

	# Dump juju status would timeout due to limited api port access
	wait_for_or_fail "! timeout 5 juju status"
	verify_instance_network_tag "${network_tag_or_group}" "10.0.0.0/24"

	echo "Temporarily allow access to the controller to unblock subsequent juju expose calls"
	allow_access_to_api_port "${instance_id}" "${region_or_az}" "${network_tag_or_group}"
	wait_for_or_fail "timeout 5 juju status"

	echo "Allow access to the controller from anywhere"
	juju expose -m controller controller --to-cidrs 0.0.0.0/0

	# Juju should be able to dump status after removing the temporary network tag
	# to avoid affecting subsequent tests.
	remove_access_to_api_port "${instance_id}" "${region_or_az}" "${network_tag_or_group}"
	wait_for_or_fail "timeout 5 juju status"
	verify_instance_network_tag "${network_tag_or_group}" "0.0.0.0/0"

	destroy_model "limit-access"
}

test_limit_access() {
	if [ -n "$(skip 'test_limit_access')" ]; then
		echo "==> SKIP: Asked to skip controller limit-access tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		case "${BOOTSTRAP_PROVIDER:-}" in
		"ec2" | "gce")
			run "run_limit_access"
			;;
		*)
			echo "==> TEST SKIPPED: test_limit_access test runs on aws/gce only"
			;;
		esac
	)
}
