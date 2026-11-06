# pack_charm uses charmcraft to pack the local charm at the given directory,
# and returns the path to the packed charm which can be supplied to juju deploy.
#
# The function returns a resolved file path so callers may safely quote the
# substitution:
#    juju deploy "$(pack_charm ./testcharms/charms/ubuntu-plus)"
#
# The unquoted form also works:
#    juju deploy $(pack_charm ./testcharms/charms/ubuntu-plus)
#
# charmcraft >= 4.4.1 may leave the packed charm inside the project
# directory instead of the current working directory (upstream bug
# canonical/charmcraft#2854), so both locations are checked below.
pack_charm() {
	local CHARM_DIR=$1 timing_id rc=0
	CHARM_NAME=$(basename "$CHARM_DIR")

	timing_start timing_id pack_charm "${CHARM_NAME}"
	charmcraft pack -p "$CHARM_DIR" >&2 || rc=$?
	if [[ ${rc} -ne 0 ]]; then
		timing_end "${timing_id}" fail
		return "${rc}"
	fi
	timing_end "${timing_id}"
	local charm_file
	charm_file=$(ls -1 ./"${CHARM_NAME}"_*.charm "${CHARM_DIR}"/"${CHARM_NAME}"_*.charm 2>/dev/null | head -n1)
	echo "${charm_file:-./${CHARM_NAME}_*.charm}"
}

pack_resource_charm() {
	local source_dir
	local output_dir=$2
	local arch=${MODEL_ARCH:-${BUILD_ARCH:-amd64}}
	local build_dir
	local packed
	local timing_id

	timing_start timing_id pack_charm "$(basename "$1")"
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
			timing_end "${timing_id}"
			echo "${packed}"
			return
		fi
	done
	timing_end "${timing_id}" fail
	return 1
}
