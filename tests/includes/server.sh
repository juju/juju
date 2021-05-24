start_server() {
	local path

	path=${1}

	(
		cd "${path}"
		python3 -m http.server 8666 >"${TEST_DIR}/server.log" 2>&1 &
		SERVER_PID=$!

		echo "${SERVER_PID}" >"${TEST_DIR}/server.pid"
	)
}

kill_server() {
	if [[ ! -f "${TEST_DIR}/server.pid" ]]; then
		return
	fi

	pid=$(cat "${TEST_DIR}/server.pid" | head -n 1 || echo "NOT FOUND")
	if [[ ${pid} == "NOT FOUND" ]]; then
		return
	fi

	echo "==> Killing server"
	kill -9 "${pid}" >/dev/null 2>&1 || true
	echo "==> Killed server (PID is $(green "${pid}"))"
}
