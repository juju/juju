test_gce_pro_image() {
	local release_name="$1"
	local release_version=""

	case "$release_name" in
	jammy) release_version="22.04" ;;
	noble) release_version="24.04" ;;
	*)
		echo "Unsupported release: $release_name"
		return 1
		;;
	esac

	echo "Testing Ubuntu Pro ${release_name} (${release_version}) image on GCE..."

	# Find the latest Ubuntu Pro image for given release
	name_filter="name~^ubuntu-pro-${release_version//./}-${release_name}- AND NOT name~arm64"
	image_id="$(gcloud compute images list \
		--project ubuntu-os-pro-cloud \
		--filter="${name_filter}" \
		--sort-by=~creationTimestamp \
		--limit=1 \
		--format=json | jq -r '.[0].selfLink | split("/") | .[-1]')"

	# Switch to the pro image stream
	juju model-config image-stream=pro

	# Add the image to custom metadata (simplestream incomplete for GCE)
	juju metadata add-image --base "ubuntu@${release_version}" "${image_id}" --stream pro

	# Add machine using this image
	juju add-machine --base "ubuntu@${release_version}" --constraints "image-id=${image_id}"

	machine_info="$(juju list-machines --format=json)"
	machine_id=$(jq -r --arg ch "$release_version" \
		'.machines | to_entries[] | select(.value.base.channel==$ch) | .key' <<<"$machine_info")

	wait_for_machine_agent_status "$machine_id" "started"

	# Refresh machine info and verify the actual instance uses the expected image
	machine_info="$(juju list-machines --format=json)"
	instance_id="$(jq -r --arg id "$machine_id" '.machines[$id]."instance-id"' <<<"$machine_info")"
	source_image_id=$(gcloud compute disks list \
		--filter="name=$instance_id" \
		--format="json" | jq -r '.[0].sourceImage | split("/")[-1]')

	test "$image_id" = "$source_image_id"
}

run_constraints_gce() {
	echo

	setup_gcloudcli_credential
	echo "==> Checking for dependencies"
	check_dependencies gcloud

	name="constraints-gce"

	file="${TEST_DIR}/constraints-gce.txt"

	ensure "${name}" "${file}"

	test_gce_pro_image noble

	destroy_model "${name}"
}
