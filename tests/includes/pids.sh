cleanup_pids() {
	if [[ -f "${TEST_DIR}/pids" ]]; then
		echo "====> Cleaning up pids"

		while read -r pid; do
			if ps -p "${pid}" >/dev/null; then
				kill -9 "${pid}" || true
			fi
		done <"${TEST_DIR}/pids"
		rm -f "${TEST_DIR}/pids" || true

	fi
	echo "====> Completed cleaning up pids"
}
