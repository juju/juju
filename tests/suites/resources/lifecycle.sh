resource_value() {
	local entity=$1
	local resource_name=$2
	local field=$3

	if [[ "${entity}" == */* ]]; then
		juju resources "${entity}" --format json |
			yq -r '.[] | select(.name == "'"${resource_name}"'") | .'"${field}"''
	else
		juju resources "${entity}" --format json |
			yq -r '.resources[] | select(.name == "'"${resource_name}"'") | .'"${field}"''
	fi
}

resource_names() {
	local entity=$1

	if [[ "${entity}" == */* ]]; then
		juju resources "${entity}" --format json | yq -r '.[].name'
	else
		juju resources "${entity}" --format json | yq -r '.resources[].name'
	fi | sort | paste -sd, -
}

check_resource_names() {
	local entity=$1
	local expected=$2
	local actual

	actual=$(resource_names "${entity}")
	test "${actual}" = "${expected}"
}

wait_for_resource_id() {
	local entity=$1
	local resource_name=$2
	local expected=$3
	local attempt=0
	local actual

	while [ "${attempt}" -lt 120 ]; do
		actual=$(resource_value "${entity}" "${resource_name}" resourceid 2>/dev/null || true)
		if [ "${actual}" = "${expected}" ]; then
			return
		fi
		echo "waiting for ${entity} resource ${resource_name}: expected ${expected}, got ${actual}"
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))
	done
	return 1
}

pack_resource_charm() {
	local source_dir
	local output_dir=$2
	local arch=${MODEL_ARCH:-${BUILD_ARCH:-amd64}}
	local build_dir
	local packed

	source_dir=$(realpath "$1")
	rm -rf "${output_dir}"
	mkdir -p "${output_dir}/source"
	output_dir=$(realpath "${output_dir}")
	build_dir="${output_dir}/source"
	cp -a "${source_dir}/." "${build_dir}"
	(
		cd "${build_dir}" || exit
		charmcraft pack --quiet --platform "${arch}"
	)
	for packed in "${build_dir}"/*_"${arch}".charm; do
		if [ -f "${packed}" ]; then
			echo "${packed}"
			return
		fi
	done
	return 1
}

run_resource_charm_transition_same_store_resource() {
	echo
	local name="resource-charm-transition-same-store-resource"
	local file="${TEST_DIR}/test-${name}.log"
	local old_app_id old_fingerprint old_revision old_charm_revision
	local new_app_id new_fingerprint new_revision new_charm_revision

	ensure "test-${name}" "${file}"

	# Revision 31 and the destination revision 26 both use resource revision 2.
	juju deploy juju-qa-test --channel 2.0/candidate --revision 31 -n 2
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"
	juju config juju-qa-test foo-file=true
	wait_for "resource line one: testing two." "$(workload_status juju-qa-test 0).message"
	wait_for "resource line one: testing two." "$(workload_status juju-qa-test 1).message"

	old_app_id=$(resource_value juju-qa-test foo-file resourceid)
	old_fingerprint=$(resource_value juju-qa-test foo-file fingerprint)
	old_revision=$(resource_value juju-qa-test foo-file revision)
	old_charm_revision=$(juju status --format json | yq -r '.applications."juju-qa-test"."charm-rev"')
	test "$(resource_value juju-qa-test/0 foo-file resourceid)" = "${old_app_id}"
	test "$(resource_value juju-qa-test/1 foo-file resourceid)" = "${old_app_id}"

	juju config juju-qa-test foo-file=false
	juju refresh juju-qa-test --channel latest/stable --revision 26
	wait_for "juju-qa-test" "$(charm_channel "juju-qa-test" "latest/stable")"
	juju config juju-qa-test foo-file=true
	wait_for "resource line one: testing two." "$(workload_status juju-qa-test 0).message"
	wait_for "resource line one: testing two." "$(workload_status juju-qa-test 1).message"

	new_app_id=$(resource_value juju-qa-test foo-file resourceid)
	new_fingerprint=$(resource_value juju-qa-test foo-file fingerprint)
	new_revision=$(resource_value juju-qa-test foo-file revision)
	new_charm_revision=$(juju status --format json | yq -r '.applications."juju-qa-test"."charm-rev"')
	test "${new_charm_revision}" != "${old_charm_revision}"
	test "${new_app_id}" != "${old_app_id}"
	test "${new_fingerprint}" = "${old_fingerprint}"
	test "${new_revision}" = "${old_revision}"
	wait_for_resource_id juju-qa-test/0 foo-file "${new_app_id}"
	wait_for_resource_id juju-qa-test/1 foo-file "${new_app_id}"
	test "$(resource_value juju-qa-test/0 foo-file resourceid)" = "${new_app_id}"
	test "$(resource_value juju-qa-test/1 foo-file resourceid)" = "${new_app_id}"

	destroy_model "test-${name}"
}

run_resource_charm_transition_pinned_upload() {
	echo
	local name="resource-charm-transition-pinned-upload"
	local file="${TEST_DIR}/test-${name}.log"
	local old_app_id old_fingerprint old_charm_revision
	local new_app_id new_fingerprint new_charm_revision

	ensure "test-${name}" "${file}"

	# Keep the pinned upload while switching between the same charm revisions.
	juju deploy juju-qa-test --channel 2.0/candidate --revision 31 -n 2 \
		--resource foo-file="./tests/suites/resources/foo-file.txt"
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"
	juju config juju-qa-test foo-file=true
	wait_for "resource line one: did the resource attach?" "$(workload_status juju-qa-test 0).message"
	wait_for "resource line one: did the resource attach?" "$(workload_status juju-qa-test 1).message"

	old_app_id=$(resource_value juju-qa-test foo-file resourceid)
	old_fingerprint=$(resource_value juju-qa-test foo-file fingerprint)
	old_charm_revision=$(juju status --format json | yq -r '.applications."juju-qa-test"."charm-rev"')
	test "$(resource_value juju-qa-test foo-file origin)" = "upload"

	juju config juju-qa-test foo-file=false
	juju refresh juju-qa-test --channel latest/stable --revision 26
	wait_for "juju-qa-test" "$(charm_channel "juju-qa-test" "latest/stable")"
	juju config juju-qa-test foo-file=true
	wait_for "resource line one: did the resource attach?" "$(workload_status juju-qa-test 0).message"
	wait_for "resource line one: did the resource attach?" "$(workload_status juju-qa-test 1).message"

	new_app_id=$(resource_value juju-qa-test foo-file resourceid)
	new_fingerprint=$(resource_value juju-qa-test foo-file fingerprint)
	new_charm_revision=$(juju status --format json | yq -r '.applications."juju-qa-test"."charm-rev"')
	test "${new_charm_revision}" != "${old_charm_revision}"
	test "${new_app_id}" != "${old_app_id}"
	test "${new_fingerprint}" = "${old_fingerprint}"
	test "$(resource_value juju-qa-test foo-file origin)" = "upload"
	wait_for_resource_id juju-qa-test/0 foo-file "${new_app_id}"
	wait_for_resource_id juju-qa-test/1 foo-file "${new_app_id}"
	test "$(resource_value juju-qa-test/0 foo-file resourceid)" = "${new_app_id}"
	test "$(resource_value juju-qa-test/1 foo-file resourceid)" = "${new_app_id}"

	destroy_model "test-${name}"
}

run_resource_charm_transition_resource_names() {
	echo
	local name="resource-charm-transition-resource-names"
	local file="${TEST_DIR}/test-${name}.log"
	local charm_v1 charm_v2 charm_v3 old_retained_id new_retained_id final_retained_id
	local retained_file="${TEST_DIR}/${name}-retained"
	local removed_file="${TEST_DIR}/${name}-removed"
	local added_file="${TEST_DIR}/${name}-added"
	local final_retained_file="${TEST_DIR}/${name}-retained-final"
	local final_added_file="${TEST_DIR}/${name}-added-final"
	local retained_digest added_digest final_retained_digest final_added_digest

	ensure "test-${name}" "${file}"
	charm_v1=$(pack_resource_charm ./tests/suites/resources/charms/resource-lifecycle-v1 "${TEST_DIR}/${name}-v1")
	charm_v2=$(pack_resource_charm ./tests/suites/resources/charms/resource-lifecycle-v2 "${TEST_DIR}/${name}-v2")
	charm_v3=$(pack_resource_charm ./tests/suites/resources/charms/resource-lifecycle-v3 "${TEST_DIR}/${name}-v3")
	printf 'retained content\n' >"${retained_file}"
	printf 'removed content\n' >"${removed_file}"
	printf 'added content\n' >"${added_file}"
	printf 'retained content after second refresh\n' >"${final_retained_file}"
	printf 'added content after second refresh\n' >"${final_added_file}"
	retained_digest=$(sha256sum "${retained_file}" | awk '{print $1}')
	added_digest=$(sha256sum "${added_file}" | awk '{print $1}')
	final_retained_digest=$(sha256sum "${final_retained_file}" | awk '{print $1}')
	final_added_digest=$(sha256sum "${final_added_file}" | awk '{print $1}')

	juju deploy "${charm_v1}" resource-lifecycle -n 2 \
		--resource retained="${retained_file}" \
		--resource removed="${removed_file}"
	wait_for "retained=${retained_digest}" "$(workload_status resource-lifecycle 0).message"
	wait_for "retained=${retained_digest}" "$(workload_status resource-lifecycle 1).message"
	check_resource_names resource-lifecycle "removed,retained"
	old_retained_id=$(resource_value resource-lifecycle retained resourceid)

	juju refresh resource-lifecycle --path "${charm_v2}" \
		--resource retained="${retained_file}" \
		--resource added="${added_file}"
	wait_for "retained=${retained_digest} added=${added_digest}" \
		"$(workload_status resource-lifecycle 0).message"
	wait_for "retained=${retained_digest} added=${added_digest}" \
		"$(workload_status resource-lifecycle 1).message"

	check_resource_names resource-lifecycle "added,retained"
	new_retained_id=$(resource_value resource-lifecycle retained resourceid)
	test "${new_retained_id}" != "${old_retained_id}"
	test "$(resource_value resource-lifecycle/0 retained resourceid)" = "${new_retained_id}"
	test "$(resource_value resource-lifecycle/1 retained resourceid)" = "${new_retained_id}"
	check_resource_names resource-lifecycle/0 "added,retained"
	check_resource_names resource-lifecycle/1 "added,retained"

	juju refresh resource-lifecycle --path "${charm_v3}" \
		--resource retained="${final_retained_file}" \
		--resource added="${final_added_file}"
	wait_for "revision=3 retained=${final_retained_digest} added=${final_added_digest}" \
		"$(workload_status resource-lifecycle 0).message"
	wait_for "revision=3 retained=${final_retained_digest} added=${final_added_digest}" \
		"$(workload_status resource-lifecycle 1).message"
	final_retained_id=$(resource_value resource-lifecycle retained resourceid)
	test "${final_retained_id}" != "${new_retained_id}"
	test "$(resource_value resource-lifecycle/0 retained resourceid)" = "${final_retained_id}"
	test "$(resource_value resource-lifecycle/1 retained resourceid)" = "${final_retained_id}"
	check_resource_names resource-lifecycle "added,retained"

	destroy_model "test-${name}"
}

run_resource_charm_transition_failed_staging() {
	echo
	local name="resource-charm-transition-failed-staging"
	local file="${TEST_DIR}/test-${name}.log"
	local charm_v1 charm_v2 charm_invalid old_retained_id old_charm_revision
	local retained_file="${TEST_DIR}/${name}-retained"
	local removed_file="${TEST_DIR}/${name}-removed"
	local added_file="${TEST_DIR}/${name}-added"
	local retained_digest refresh_output

	ensure "test-${name}" "${file}"
	charm_v1=$(pack_resource_charm ./tests/suites/resources/charms/resource-lifecycle-v1 "${TEST_DIR}/${name}-v1")
	charm_v2=$(pack_resource_charm ./tests/suites/resources/charms/resource-lifecycle-v2 "${TEST_DIR}/${name}-v2")
	charm_invalid=$(pack_resource_charm ./tests/suites/resources/charms/resource-lifecycle-invalid "${TEST_DIR}/${name}-invalid")
	printf 'retained content\n' >"${retained_file}"
	printf 'removed content\n' >"${removed_file}"
	printf 'added content\n' >"${added_file}"
	retained_digest=$(sha256sum "${retained_file}" | awk '{print $1}')

	juju deploy "${charm_v1}" resource-lifecycle \
		--resource retained="${retained_file}" \
		--resource removed="${removed_file}"
	wait_for "resource-lifecycle" "$(idle_condition "resource-lifecycle")"
	old_retained_id=$(resource_value resource-lifecycle retained resourceid)
	old_charm_revision=$(juju status --format json | yq -r '.applications."resource-lifecycle"."charm-rev"')

	# Stage a repository resource without uploading a file. Uploading a local
	# file here is incorrectly matched to the destination OCI resource.
	if refresh_output=$(juju refresh resource-lifecycle --path "${charm_invalid}" \
		--resource added=1 2>&1); then
		echo "refresh with an incompatible retained resource unexpectedly succeeded"
		return 1
	fi
	check_contains "${refresh_output}" 'cannot reuse resource "retained" when its type changes'

	test "$(juju status --format json | yq -r '.applications."resource-lifecycle"."charm-rev"')" = \
		"${old_charm_revision}"
	test "$(resource_value resource-lifecycle retained resourceid)" = "${old_retained_id}"
	check_resource_names resource-lifecycle "removed,retained"
	wait_for "retained=${retained_digest}" "$(workload_status resource-lifecycle 0).message"

	juju refresh resource-lifecycle --path "${charm_v2}" \
		--resource retained="${retained_file}" \
		--resource added="${added_file}"
	wait_for "resource-lifecycle" "$(idle_condition "resource-lifecycle")"
	check_resource_names resource-lifecycle "added,retained"
	test "$(resource_value resource-lifecycle retained resourceid)" != "${old_retained_id}"

	destroy_model "test-${name}"
}

run_resource_get_lifecycle() {
	echo
	local name="resource-get-lifecycle"
	local file="${TEST_DIR}/test-${name}.log"
	local charm resource_one resource_two digest_one digest_two

	ensure "test-${name}" "${file}"
	charm=$(pack_resource_charm ./tests/suites/resources/charms/resources "${TEST_DIR}/${name}-charm")
	resource_one="${TEST_DIR}/${name}-one"
	resource_two="${TEST_DIR}/${name}-two"
	printf 'first resource-get content\n' >"${resource_one}"
	printf 'second resource-get content\n' >"${resource_two}"
	digest_one=$(sha256sum "${resource_one}" | awk '{print $1}')
	digest_two=$(sha256sum "${resource_two}" | awk '{print $1}')

	juju deploy "${charm}" resources --resource test-resource="${resource_one}"
	wait_for "test-resource: $(wc -c <"${resource_one}") bytes, sha256 ${digest_one}" \
		"$(workload_status resources 0).message"

	juju attach-resource resources test-resource="${resource_two}"
	wait_for "test-resource: $(wc -c <"${resource_two}") bytes, sha256 ${digest_two}" \
		"$(workload_status resources 0).message"

	juju add-unit resources
	wait_for "test-resource: $(wc -c <"${resource_two}") bytes, sha256 ${digest_two}" \
		"$(workload_status resources 1).message"
	test "$(resource_value resources/0 test-resource resourceid)" = \
		"$(resource_value resources test-resource resourceid)"
	test "$(resource_value resources/1 test-resource resourceid)" = \
		"$(resource_value resources test-resource resourceid)"

	destroy_model "test-${name}"
}

test_resource_lifecycle() {
	if [ "$(skip 'test_resource_lifecycle')" ]; then
		echo "==> TEST SKIPPED: Resource lifecycle"
		return
	fi

	(
		set_verbosity
		check_dependencies charmcraft

		cd .. || exit

		run "run_resource_charm_transition_same_store_resource"
		run "run_resource_charm_transition_pinned_upload"
		run "run_resource_charm_transition_resource_names"
		run "run_resource_charm_transition_failed_staging"
		run "run_resource_get_lifecycle"
	)
}
