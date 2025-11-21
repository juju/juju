# allow_access_to_api_port <instance_id> <zone> <network_tag>
#
# Grants temporary access to the controller's API port (17070) for the controller instance
allow_access_to_api_port() {
	local instance_id="$1" zone="$2" network_tag="$3"
	# Create a firewall rule or security group via the provider specific tool
	# to allow api port access
	instance_ip=$(curl -s https://checkip.amazonaws.com)
	case "${BOOTSTRAP_PROVIDER:-}" in
	"ec2")
		aws ec2 authorize-security-group-ingress \
			--group-name "${network_tag}" \
			--protocol tcp \
			--port 17070 \
			--cidr "${instance_ip}"/32
		;;
	"gce")
		temp_network_tag="temp-${network_tag}"
		gcloud compute instances add-tags "${instance_id}" \
			--zone="${zone}" \
			--tags="${temp_network_tag}"
		gcloud compute firewall-rules create "${instance_id}-${temp_network_tag}" \
			--direction=INGRESS \
			--priority=1000 \
			--network=default \
			--action=ALLOW \
			--rules=tcp:17070 \
			--source-ranges="${instance_ip}"/32 \
			--target-tags="${temp_network_tag}"
		;;
	*)
		echo "Aborting: adding temporary access to the API port is not supported for the provider: ${BOOTSTRAP_PROVIDER}"
		exit 1
		;;
	esac
}

# remove_access_to_api_port <instance_id> <zone> <network_tag>
#
# Revokes the temporary access previously granted to the controller's API port (17070).
remove_access_to_api_port() {
	local instance_id="$1" zone="$2" network_tag="$3"
	# Create a firewall rule via the provider specific tool to allow api port access
	instance_ip=$(curl -s https://checkip.amazonaws.com)
	case "${BOOTSTRAP_PROVIDER:-}" in
	"ec2")
		aws ec2 revoke-security-group-ingress \
			--group-name "${network_tag}" \
			--protocol tcp \
			--port 17070 \
			--cidr "${instance_ip}"/32
		;;
	"gce")
		temp_network_tag="temp-${network_tag}"
		gcloud compute instances remove-tags "${instance_id}" \
			--zone="${zone}" \
			--tags="${temp_network_tag}"
		gcloud compute firewall-rules delete "${instance_id}-${temp_network_tag}" \
			--quiet
		;;
	*)
		echo "Aborting: removing temporary access to the API port is not supported for the provider: ${BOOTSTRAP_PROVIDER}"
		exit 1
		;;
	esac
}

# region_or_availability_zone
#
# Returns the region or availability zone of the controller model depends on the model
region_or_availability_zone() {
	case "${BOOTSTRAP_PROVIDER:-}" in
	"ec2")
		juju show-model controller --format json | jq -r '.["controller"]["region"]'
		;;
	"gce")
		juju show-machine -m controller 0 --format=json | jq -r '.["machines"]["0"]["hardware"]' | grep -oP 'availability-zone=\K\S+'
		;;
	*)
		echo "Aborting, we shouldn't be here"
		exit 1
		;;
	esac
}

# instance_network_tag_or_group
#
# Returns the identifier used to target the controller instance for firewall/security rules.
instance_network_tag_or_group() {
	case "${BOOTSTRAP_PROVIDER:-}" in
	"ec2")
		model_uuid=$(juju show-model controller --format json | jq -r '.["controller"]["model-uuid"]')
		echo "juju-${model_uuid}-0"
		;;
	"gce")
		machine_info="$(juju list-machines -m controller --format=json)"
		echo "$(jq -r '.machines["0"]."instance-id"' <<<"$machine_info")"
		;;
	*)
		echo "Aborting, we shouldn't be here"
		exit 1
		;;
	esac
}
