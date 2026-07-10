run_check_go_version() {
	OUT=$(check_go_version 2>&1 || true)
	if [ -n "${OUT}" ]; then
		echo ""
		echo "$(red 'Found some issues:')"
		echo "${OUT}"
		exit 1
	fi
}

check_go_version() {
	exit_code=0
	target_version="$(go mod edit -json | yq -r .Go | awk 'BEGIN{FS="."} {print $1"."$2}')"
	target_minor_version="$(go mod edit -json | yq -r .Go | awk 'BEGIN{FS="."} {print $1"."$2".0"}')"

	check_gotoolchain() {
		local yaml="$1"
		local part="$2"
		local label="$3"
		local build_steps
		build_steps="$(yq -r '.parts | .["'"${part}"'"] | .override-build' "${yaml}")"
		echo "${build_steps}" | grep -q "GOTOOLCHAIN=go${target_minor_version}+auto"
		if [ $? -ne 0 ]; then
			echo "Go version in go.mod (${target_version}) does not match snapcraft.yaml GOTOOLCHAIN value for ${label}"
			exit_code=1
		fi
	}

	check_gotoolchain "snaps/juju/snapcraft.yaml" "juju" "juju"
	check_gotoolchain "snaps/juju/snapcraft.yaml" "jujuagentd" "jujuagentd"
	check_gotoolchain "snaps/jujud/snapcraft.yaml" "jujud" "jujud"

	exit "${exit_code}"
}

run_check_juju_version() {
	OUT=$(check_juju_version 2>&1 || true)
	if [ -n "${OUT}" ]; then
		echo ""
		echo "$(red 'Found some issues:')"
		echo "${OUT}"
		exit 1
	fi
}

check_juju_version() {
	target_version="$(go run scripts/version/main.go)"
	snapcraft_yaml="snaps/juju/snapcraft.yaml"

	snapcraft_juju_version="$(yq -r '.version' "${snapcraft_yaml}")"
	echo "${snapcraft_juju_version}" | grep -q "${target_version}"
	if [ $? -ne 0 ]; then
		echo "Juju version in version/version.go (${target_version}) does not match snapcraft.yaml (${snapcraft_juju_version}) for juju"
		exit_code=1
	fi

	win_installer_juju_version="$(cat scripts/win-installer/setup.iss | sed -n 's/.*MyAppVersion="\(.*\)".*/\1/p')"
	echo "${win_installer_juju_version}" | grep -q "${target_version}"
	if [ $? -ne 0 ]; then
		echo "Juju version in version/version.go (${target_version}) does not match setup.iss (${win_installer_juju_version}) for juju"
		exit_code=1
	fi
}

test_versions() {
	if [ "$(skip 'test_versions')" ]; then
		echo "==> TEST SKIPPED: versions"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run_linter "run_check_go_version"
		run_linter "run_check_juju_version"
	)
}
