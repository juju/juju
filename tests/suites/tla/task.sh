tla_tools_sha256() {
	local version
	version=${1}

	case "${version}" in
	v1.8.0) echo "71546dff3897a01b0ee4fa64135d9f5e9384d2b7e47b3cc20a16b655b0eb4f86" ;;
	*)
		echo "No known sha256 for tla2tools ${version}; set TLA_TOOLS_SHA256" >&2
		return 1
		;;
	esac
}

verify_tla_tools_sha256() {
	local file expected_sha got_sha

	file=${1}
	expected_sha=${2}
	got_sha=$(sha256sum "${file}" | awk '{print $1}')

	if [ "${got_sha}" != "${expected_sha}" ]; then
		echo "sha256sum mismatch (${got_sha}, expected ${expected_sha})"
		return 1
	fi
}

download_tla_tools() {
	local url output version

	url=${1}
	output=${2}
	version=${3}

	mkdir -p "$(dirname "${output}")"
	echo "Downloading TLA+ tools (${version})"
	curl --fail --location --silent --show-error \
		"${url}" \
		--output "${output}"
}

run_prepare_tla_tools() {
	local project_dir tla_tools_version tla_tools_dir tla_tools_jar tla_tools_url
	local expected_tla_tools_sha256

	project_dir=$(pwd)

	tla_tools_version="${TLA_TOOLS_VERSION:-v1.8.0}"
	tla_tools_dir="${project_dir}/.cache/tla2tools/${tla_tools_version}"
	tla_tools_jar="${TLC_JAR:-${tla_tools_dir}/tla2tools.jar}"
	tla_tools_url="https://github.com/tlaplus/tlaplus/releases/download/${tla_tools_version}/tla2tools.jar"
	expected_tla_tools_sha256="${TLA_TOOLS_SHA256:-$(tla_tools_sha256 "${tla_tools_version}")}"

	if [ ! -f "${tla_tools_jar}" ]; then
		download_tla_tools "${tla_tools_url}" "${tla_tools_jar}" "${tla_tools_version}"
	fi

	if ! verify_tla_tools_sha256 "${tla_tools_jar}" "${expected_tla_tools_sha256}"; then
		rm -f "${tla_tools_jar}"
		download_tla_tools "${tla_tools_url}" "${tla_tools_jar}" "${tla_tools_version}"
		verify_tla_tools_sha256 "${tla_tools_jar}" "${expected_tla_tools_sha256}" || exit 1
	fi
}

run_tla_model() {
	local spec_path cfg_path
	local project_dir spec_file cfg_file
	local tla_tools_version tla_tools_dir tla_tools_jar
	local work_dir

	spec_path=${1}
	cfg_path=${2}

	project_dir=$(pwd)
	spec_file="${project_dir}/${spec_path}"
	cfg_file="${project_dir}/${cfg_path}"

	tla_tools_version="${TLA_TOOLS_VERSION:-v1.8.0}"
	tla_tools_dir="${project_dir}/.cache/tla2tools/${tla_tools_version}"
	tla_tools_jar="${TLC_JAR:-${tla_tools_dir}/tla2tools.jar}"

	if [ ! -f "${tla_tools_jar}" ]; then
		echo "tla2tools.jar not found; run run_prepare_tla_tools first" >&2
		exit 1
	fi

	if [ ! -f "${spec_file}" ]; then
		echo "TLA spec not found: ${spec_file}" >&2
		exit 1
	fi

	if [ ! -f "${cfg_file}" ]; then
		echo "TLA config not found: ${cfg_file}" >&2
		exit 1
	fi

	work_dir=$(mktemp -d "${TEST_DIR}/tla.XXX")
	# shellcheck disable=SC2064
	trap "rm -rf '${work_dir}'" RETURN

	cp "${spec_file}" "${cfg_file}" "${work_dir}/"

	(
		cd "${work_dir}" || exit
		java -XX:+UseParallelGC -cp "${tla_tools_jar}" tlc2.TLC \
			-cleanup \
			-workers auto \
			-config "$(basename "${cfg_file}")" \
			"$(basename "${spec_file}")"
	)
}

run_migration_transition_phases() {
	run_tla_model \
		"core/migration/tla/MigrationTransitionPhases.tla" \
		"core/migration/tla/MigrationTransitionPhases.cfg"
}

run_objectstore_transition_phases() {
	run_tla_model \
		"core/objectstore/tla/ObjectStoreTransitionPhases.tla" \
		"core/objectstore/tla/ObjectStoreTransitionPhases.cfg"
}

test_tla() {
	if [ "$(skip 'test_tla')" ]; then
		echo "==> TEST SKIPPED: tla checks"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		echo "==> Checking for dependencies"
		check_dependencies java curl sha256sum

		run "run_prepare_tla_tools"
		run "run_migration_transition_phases"
		run "run_objectstore_transition_phases"
	)
}
