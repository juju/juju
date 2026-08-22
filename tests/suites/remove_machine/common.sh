cloud_instance_removal_supported() {
	case "${BOOTSTRAP_PROVIDER:-}" in
	"lxd")
		[[ ${BOOTSTRAP_CLOUD:-localhost} == "localhost" ]]
		return
		;;
	*)
		return 1
		;;
	esac
}

delete_cloud_instance() {
	local model machine_id instance_id project

	model=${1}
	machine_id=${2}
	instance_id=$(juju show-machine -m "${model}" "${machine_id}" --format=json |
		yq -r ".machines.\"${machine_id}\".\"instance-id\"")
	if [[ -z ${instance_id} || ${instance_id} == "null" || ${instance_id} == "pending" ]]; then
		echo "could not determine instance-id for machine ${machine_id} in model ${model}"
		return 1
	fi

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
	*)
		echo "cloud instance removal is not supported for ${BOOTSTRAP_PROVIDER:-unknown}"
		return 1
		;;
	esac
}
