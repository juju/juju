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
#
# Packing is serialised within a run, as charmcraft is not safe to run
# concurrently. Inside a concurrent subtest (run_concurrently) each call
# returns its own copy of the charm, so subtests packing the same charm
# cannot overwrite a file another one is deploying.
pack_charm() {
	if [[ -z ${TEST_DIR:-} ]]; then
		_pack_charm "$@"
		return
	fi

	local packed copy
	# The lock is opened inside the substitution: a redirection on an
	# assignment is applied only after the substitution has run.
	packed=$(
		exec 9>"${TEST_DIR}/.pack-charm.lock"
		flock 9
		_pack_charm "$@"
	) || return
	if [[ -z ${SUBTEST_SCRATCH_DIR:-} || ! -f ${packed} ]]; then
		echo "${packed}"
		return
	fi
	copy=$(mktemp -d -p "${SUBTEST_SCRATCH_DIR}" charm.XXXXXX)
	cp "${packed}" "${copy}/"
	echo "${copy}/$(basename "${packed}")"
}

# _pack_charm packs a charm and prints its path (see pack_charm).
_pack_charm() {
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
		# Serialised with pack_charm: charmcraft is not safe to run
		# concurrently.
		if [[ -n ${TEST_DIR:-} ]]; then
			exec 9>"${TEST_DIR}/.pack-charm.lock"
			flock 9
		fi
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
