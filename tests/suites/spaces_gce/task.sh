test_spaces_gce() {
	if [ "$(skip 'test_spaces_gce')" ]; then
		echo "==> TEST SKIPPED: space tests (GCE)"
		return
	fi

	setup_gcloudcli_credential

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju gcloud

	if [ "${BOOTSTRAP_REGION:-}" == "" ]; then
		BOOTSTRAP_REGION=us-east1
	fi

	echo "==> Ensure subnet for alternative space exists"
	network_name="juju-qa-test"
	ensure_subnets $network_name "${BOOTSTRAP_REGION}"

	file="${TEST_DIR}/test-spaces-gce.log"

	export BOOTSTRAP_ADDITIONAL_ARGS="${BOOTSTRAP_ADDITIONAL_ARGS:-} --model-default=vpc-id=juju-qa-test"
	bootstrap "test-spaces-gce" "${file}"

	test_machines_in_spaces

	destroy_controller "test-spaces-gce"
}

ensure_subnets() {
	local network_name=$1
	local region=$2
	existing_network=$(gcloud compute networks list --format json | jq -r '.[] | select(.name=="'"${network_name}"'") | .name')
	if [ "$existing_network" == "" ]; then
		echo "Creating VPC ${network_name} in region ${region}"
		gcloud compute networks create "$network_name" --subnet-mode="custom" --description "test vpc for juju qa"
	fi

	icmp_rule_name="juju-qa-test-allow-icmp"
	existing_icmp_firewall=$(gcloud compute firewall-rules list --format json | jq -r '.[] | select(.network | endswith("networks/'"${network_name}"'")) | select(.name == "'"${icmp_rule_name}"'") | .name')
	if [ "$existing_icmp_firewall" == "" ]; then
		echo "Creating ICMP rule ${icmp_rule_name}"
		gcloud compute firewall-rules create "$icmp_rule_name" --network "${network_name}" --allow="icmp" --source-ranges="0.0.0.0/0"
	fi

	ssh_rule_name="juju-qa-test-allow-ssh"
	existing_ssh_firewall=$(gcloud compute firewall-rules list --format json | jq -r '.[] | select(.network | endswith("networks/'"${network_name}"'")) | select(.name == "'"${ssh_rule_name}"'") | .name')
	if [ "$existing_ssh_firewall" == "" ]; then
		echo "Creating SSH rule ${ssh_rule_name}"
		gcloud compute firewall-rules create "$ssh_rule_name" --network "${network_name}" --allow="tcp:22" --source-ranges="0.0.0.0/0"
	fi

	for i in "subnet1 10.104.0.0/20" "subnet2 10.142.0.0/20"; do
		set -- $i
		existing_range=$(gcloud compute networks subnets list --regions "${region}" --format json | jq -r '.[] | select(.network | endswith("networks/'"${network_name}"'")) | select(.ipCidrRange=="'"${2}"'") | .ipCidrRange')
		if [ "$existing_range" == "" ]; then
			echo "Creating subnet $1 with CIDR range $2"
			gcloud compute networks subnets create "${1}" --region "${region}" --network "${network_name}" --range "${2}"
		fi
	done
}
