add_clean_func() {
	local name

	name=${1}

	echo "${name}" >>"${TEST_DIR}/cleanup"
}

# cleanup_funcs attempts to clean up with functions.
cleanup_funcs() {
	local failed=0
	if [[ -f "${TEST_DIR}/cleanup" ]]; then
		while read -r CMD; do
			echo "====> Running clean up func: ${CMD}"
			if ! $CMD; then
				echo "====> Clean up func failed: ${CMD}" >&2
				failed=1
			fi
			echo "====> Finished cleaning up func: ${CMD}"
		done <"${TEST_DIR}/cleanup"
	fi
	return "${failed}"
}
