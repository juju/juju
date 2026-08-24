test_gce_pro_image() {
	local release_name="$1"
	local release_version=""
	local name file

	case "$release_name" in
	jammy) release_version="22.04" ;;
	noble) release_version="24.04" ;;
	*)
		echo "Unsupported release: $release_name"
		return 1
		;;
	esac

	name="constraints-gce-pro-image-${release_name}"
	file="${TEST_DIR}/constraints-gce-pro-image-${release_name}.txt"
	ensure "${name}" "${file}"

	echo "Testing Ubuntu Pro ${release_name} (${release_version}) image on GCE..."

	# Find the latest Ubuntu Pro image for given release
	name_filter="name~^ubuntu-pro-${release_version//./}-${release_name}- AND NOT name~arm64"
	image_id="$(gcloud compute images list \
		--project ubuntu-os-pro-cloud \
		--filter="${name_filter}" \
		--sort-by=~creationTimestamp \
		--limit=1 \
		--format=json | yq -r '.[0].selfLink | split("/") | .[-1]')"

	# Switch to the pro image stream
	juju model-config image-stream=pro

	# Add the image to custom metadata (simplestream incomplete for GCE)
	juju metadata add-image --base "ubuntu@${release_version}" "${image_id}" --stream pro

	# Add machine using this image
	juju add-machine --base "ubuntu@${release_version}" --constraints "image-id=${image_id}"

	machine_info="$(juju list-machines --format=json)"
	machine_id=$(ch="$release_version" yq -r \
		'.machines | to_entries[] | select(.value.base.channel==env(ch)) | .key' <<<"$machine_info")

	wait_for_machine_agent_status "$machine_id" "started"

	# Refresh machine info and verify the actual instance uses the expected image
	machine_info="$(juju list-machines --format=json)"
	instance_id="$(id="$machine_id" yq -r '.machines[env(id)]."instance-id"' <<<"$machine_info")"
	source_image_id=$(gcloud compute disks list \
		--filter="name=$instance_id" \
		--format="json" | yq -r '.[0].sourceImage | split("/") | .[-1]')

	test "$image_id" = "$source_image_id"

	destroy_model "${name}"
}

test_gce_image_id_constraint() {
	local name file

	name="constraints-gce-image-id"
	file="${TEST_DIR}/constraints-gce-image-id.txt"
	ensure "${name}" "${file}"

	echo "Testing image-id constraint with a custom GCE image..."

	add_clean_func "run_cleanup_gce_image"

	local project_id custom_image image_ref
	project_id="$(gcloud config get-value project)"
	custom_image="$(create_gce_image_and_wait_available)"
	image_ref="projects/${project_id}/global/images/${custom_image}"

	echo "Deploy 2 machines with different constraints"
	juju add-machine --constraints "cores=2"
	juju add-machine --constraints "image-id=${image_ref}"

	wait_for_machine_agent_status "0" "started"
	wait_for_machine_agent_status "1" "started"

	echo "Ensure machine 0 has 2 cores"
	machine0_hardware="$(juju machines --format json | yq -r '.["machines"]["0"]["hardware"]')"
	check_contains "${machine0_hardware}" "cores=2"

	echo "Ensure machine 1 uses the correct image ID from image-id constraint"
	instance_id="$(juju show-machine 1 --format json | yq -r '.["machines"]["1"]["instance-id"]')"
	az="$(juju show-machine 1 --format yaml | yq -r '.machines["1"].hardware' | tr ' ' '\n' | grep 'availability-zone' | cut -d= -f2)"
	source_image_id="$(gcloud compute disks describe "${instance_id}" --zone "${az}" --format='value(sourceImage)' | awk -F/ '{print $NF}')"

	echo "${source_image_id}" | check "${custom_image}"

	destroy_model "${name}"
}

run_constraints_gce() {
	echo

	setup_gcloudcli_credential
	echo "==> Checking for dependencies"
	check_dependencies gcloud

	test_gce_pro_image noble
	test_gce_image_id_constraint
}
