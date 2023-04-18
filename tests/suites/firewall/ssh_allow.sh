run_firewall_ssh() {
	echo

	file="${TEST_DIR}/network-health.txt"

	ensure "firewall-ssh" "${file}"

	juju add-machine
	wait_for_machine_agent_status "0" "started"

	echo "==> Verifying default setting"
	juju model-config ssh-allow | check "0.0.0.0/0,::/0"
	model_uuid=$(juju show-model --format json | jq -r '.["firewall-ssh"]["model-uuid"]')
	secgroup=$(aws ec2 describe-security-groups | jq -r ".SecurityGroups[] | select(.GroupName == \"juju-${model_uuid}\")")
	echo $secgroup | jq -r ".IpPermissions[] | select(.FromPort == 22) | .IpRanges[0].CidrIp" | check "0.0.0.0/0"
	echo $secgroup | jq -r ".IpPermissions[] | select(.FromPort == 22) | .Ipv6Ranges[0].CidrIpv6" | check "::/0"

	echo "==> Verifying changed setting"
	juju model-config ssh-allow="192.168.0.0/24"
	attempt=0
	while true; do
		secgroup=$(aws ec2 describe-security-groups | jq -r ".SecurityGroups[] | select(.GroupName == \"juju-${model_uuid}\")")
		ingress=$(echo $secgroup | jq -r ".IpPermissions[] | select(.FromPort == 22) | .IpRanges[0].CidrIp")
		ingressv6=$(echo $secgroup | jq -r ".IpPermissions[] | select(.FromPort == 22) | .IpRanges[0].CidrIpv6")
		if [ "${ingress}" == "192.168.0.0/24" ] && [ "${ingressv6}" == "null" ]; then
			break
		fi
		if [ $attempt -eq 5 ]; then
			echo "$(red 'timeout: waiting for ssh allow to update in aws')"
		fi
		attempt=$((attempt + 1))
		sleep 1
	done
}

test_firewall_ssh() {
	if [ "$(skip 'test_firewall_ssh')" ]; then
		echo "==> TEST SKIPPED: test_firewall_ssh"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_firewall_ssh"
	)
}
