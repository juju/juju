cloud_instance_removal_supported() {
	case "${BOOTSTRAP_PROVIDER:-}" in
	"lxd")
		return 0
		;;
	"aws" | "ec2" | "google" | "gce" | "azure")
		return 0
		;;
	*)
		return 1
		;;
	esac
}

delete_cloud_instance() {
	local model machine_id instance_id availability_zone project region resource_group

	if ! cloud_instance_removal_supported; then
		echo "cloud instance removal is not supported for ${BOOTSTRAP_PROVIDER:-unknown}"
		return 1
	fi

	model=${1}
	machine_id=${2}
	instance_id=$(juju show-machine -m "${model}" "${machine_id}" --format=json |
		yq -r ".machines.\"${machine_id}\".\"instance-id\"")
	if [[ -z ${instance_id} || ${instance_id} == "null" || ${instance_id} == "pending" ]]; then
		echo "could not determine instance-id for machine ${machine_id} in model ${model}"
		return 1
	fi

	echo "deleting instance ${instance_id} from cloud ${BOOTSTRAP_PROVIDER}"

	case "${BOOTSTRAP_PROVIDER:-}" in
	"lxd")
		project=$(juju model-config -m "${model}" project)
		if [[ -z ${project} ]]; then
			echo "could not determine LXD project for model ${model}"
			return 1
		fi
		lxc info "local:${instance_id}" --project "${project}" >/dev/null
		lxc delete --force "local:${instance_id}" --project "${project}"
		;;
	"aws" | "ec2")
		region=$(juju show-model "${model}" --format=json | yq -r '.[].region')
		if [[ -z ${region} || ${region} == "null" ]]; then
			echo "could not determine region for EC2 instance ${instance_id}"
			return 1
		fi
		AWS_DEFAULT_REGION="${region}" aws ec2 terminate-instances \
			--instance-ids "${instance_id}" >/dev/null
		AWS_DEFAULT_REGION="${region}" aws ec2 wait instance-terminated \
			--instance-ids "${instance_id}"
		;;
	"google" | "gce")
		availability_zone=$(juju show-machine -m "${model}" "${machine_id}" --format=json |
			machine_id="${machine_id}" yq -r '.machines[env(machine_id)].hardware' |
			grep -oP 'availability-zone=\K\S+')
		if [[ -z ${availability_zone} ]]; then
			echo "could not determine availability zone for GCE instance ${instance_id}"
			return 1
		fi
		gcloud compute instances delete "${instance_id}" \
			--zone="${availability_zone}" --quiet
		;;
	"azure")
		resource_group=$(az vm list --output=yaml |
			instance_id="${instance_id}" yq -r \
				'.[] | select(.name == env(instance_id)) | .resourceGroup')
		if [[ -z ${resource_group} || ${resource_group} == "null" ]]; then
			echo "could not determine resource group for Azure instance ${instance_id}"
			return 1
		fi
		az vm delete --resource-group "${resource_group}" \
			--name "${instance_id}" --yes
		;;
	*)
		return 1
		;;
	esac
}
