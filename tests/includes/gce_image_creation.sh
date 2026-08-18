create_gce_image_and_wait_available() {
	local source_image_family source_image_project image_name
	source_image_family="${1:-ubuntu-2404-lts-amd64}"
	source_image_project="${2:-ubuntu-os-cloud}"

	image_name="juju-constraints-$(date +%s)-$RANDOM"

	gcloud compute images create "${image_name}" \
		--source-image-family "${source_image_family}" \
		--source-image-project "${source_image_project}" \
		--quiet >/dev/null

	echo "${image_name}" >>"${TEST_DIR}/gce-images"
	echo "${image_name}"
}

run_cleanup_gce_image() {
	set +e

	if [[ -f "${TEST_DIR}/gce-images" ]]; then
		echo "====> Cleaning up GCE images"
		while read -r gce_image; do
			gcloud compute images delete "${gce_image}" --quiet >>"${TEST_DIR}/gcloud_cleanup"
		done <"${TEST_DIR}/gce-images"
	fi

	set_verbosity

	echo "====> Completed cleaning up gce images"
}
