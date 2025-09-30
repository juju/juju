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
	target_version="$(go mod edit -json | jq -r .Go | awk 'BEGIN{FS="."} {print $1"."$2}')"

	snapcraft_go_juju_version="$(yq -r '.parts | .["juju"] | .["build-snaps"].[] | select(test("go\/"))' snap/snapcraft.yaml | awk -F'/' '{print $2}')"
	echo "${snapcraft_go_juju_version}" | grep -q "${target_version}"
	if [ $? -ne 0 ]; then
		echo "Go version in go.mod (${target_version}) does not match snapcraft.yaml (${snapcraft_go_juju_version}) for juju"
		exit_code=0 # TODO(hpidcock): change back to 1 when 1.25 snap is released
	fi

	snapcraft_go_jujud_version="$(yq -r '.parts | .["jujud"] | .["build-snaps"].[] | select(test("go\/"))' snap/snapcraft.yaml | awk -F'/' '{print $2}')"
	echo "${snapcraft_go_jujud_version}" | grep -q "${target_version}"
	if [ $? -ne 0 ]; then
		echo "Go version in go.mod (${target_version}) does not match snapcraft.yaml (${snapcraft_go_jujud_version}) for jujud"
		exit_code=0 # TODO(hpidcock): change back to 1 when 1.25 snap is released
	fi

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

	snapcraft_juju_version="$(yq -r '.version' snap/snapcraft.yaml)"
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
