run_simplestream_metadata() {
	VERSION=$(jujud version)
	JUJUD_VERSION=$(jujud_version)
	echo "===> Using jujud version ${JUJUD_VERSION}"

	add_clean_func "remove_bootstrap_tools"
	add_bootstrap_tools "${VERSION}"

	add_clean_func "remove_bootstrap_metadata"
	juju metadata generate-agent-binaries \
		--clean \
		--prevent-fallback \
		-d "./tests/suites/bootstrap/streams/"

	add_clean_func "kill_server"
	start_server "./tests/suites/bootstrap/streams/tools"

	# Find a routable address to the server that isn't the loopback address.
	# Unfortunately, you can't cleanly look at the addresses and select the
	# right one.
	addresses=$(hostname -I)
	server_address=""
	for address in $(echo "${addresses}" | tr ' ' '\n'); do
		# shellcheck disable=SC2015
		curl "http://${address}:8666" >/dev/null 2>&1 && server_address="${address}" && break || true
	done

	name="test-bootstrap-stream"

	file="${TEST_DIR}/test-bootstrap-stream.log"
	juju bootstrap "lxd" "${name}" \
		--show-log \
		--config agent-metadata-url="http://${server_address}:8666/" \
		--config test-mode=true \
		--add-model=default \
		--bootstrap-base="${BOOTSTRAP_BASE}" \
		--agent-version="${JUJUD_VERSION}" 2>&1 | OUTPUT "${file}"
	echo "${name}" >>"${TEST_DIR}/jujus"

	juju deploy jameinel-ubuntu-lite
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"
}

test_bootstrap_simplestream() {
	if [ -n "$(skip 'test_bootstrap_simplestream')" ]; then
		echo "==> SKIP: Asked to skip stream tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_simplestream_metadata"
	)
}

add_bootstrap_tools() {
	local version jujud_path

	version=${1}

	jujud_path=$(which jujud)
	cp "${jujud_path}" "${TEST_DIR}"
	cd "${TEST_DIR}" || exit

	tar -zcvf "juju-${version}.tgz" jujud >/dev/null
	cd "${CURRENT_DIR}/.." || exit

	mkdir -p "./tests/suites/bootstrap/streams/tools/released/"
	mv "${TEST_DIR}/juju-${version}.tgz" "./tests/suites/bootstrap/streams/tools/released"
}

remove_bootstrap_tools() {
	cd "${CURRENT_DIR}/.." || exit

	echo "==> Removing tools"
	rm -rf ./tests/suites/bootstrap/streams/tools/released || true
	echo "==> Removed tools"
}

remove_bootstrap_metadata() {
	cd "${CURRENT_DIR}/.." || exit

	echo "==> Removing metadata"
	rm -rf ./tests/suites/bootstrap/streams/tools/streams || true
	echo "==> Removed metadata"
}
