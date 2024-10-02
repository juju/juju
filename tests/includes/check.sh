check_dependencies() {
	local dep missing
	missing=""

	for dep in "$@"; do
		if ! which "$dep" >/dev/null 2>&1; then
			[[ "$missing" ]] && missing="$missing, $dep" || missing="$dep"
		fi
	done

	if [[ "$missing" ]]; then
		echo "Missing dependencies: $missing" >&2
		echo ""
		return 1
	fi
}

check_juju_dependencies() {
	local dep missing
	missing=""

	for dep in "$@"; do
		if ! juju "$dep" >/dev/null 2>&1; then
			[[ "$missing" ]] && missing="$missing, juju $dep" || missing="juju $dep"
		fi
	done

	if [[ "$missing" ]]; then
		echo "Missing juju commands: $missing" >&2
		echo ""
		return 1
	fi
}

check_not_contains() {
	local input value chk

	input=${1}
	shift

	value=${1}
	shift

	chk=$(echo "${input}" | grep "${value}" || true)
	if [[ -n ${chk} ]]; then
		printf "Unexpected \"${value}\" found\n\n%s\n" "${input}" >&2
		return 1
	else
		echo "Success: \"${value}\" not found" >&2
	fi
}

check_contains() {
	local input value chk

	input=${1}
	shift

	value=${1}
	shift

	chk=$(echo "${input}" | grep "${value}" || true)
	if [[ -z ${chk} ]]; then
		printf "Expected \"%s\" not found\n\n%s\n" "${value}" "${input}" >&2
		return 1
	else
		echo "Success: \"${value}\" found" >&2
	fi
}

check_gt() {
	local input value chk

	input=${1}
	shift

	value=${1}
	shift

	if [[ ${input} > ${value} ]]; then
		printf "Success: \"%s\" > \"%s\"\n" "${input}" "${value}" >&2
	else
		printf "Expected \"%s\" > \"%s\"\n" "${input}" "${value}" >&2
		return 1
	fi
}

check_ge() {
	local input value chk

	input=${1}
	shift

	value=${1}
	shift

	if [[ ${input} > ${value} ]] || [ "${input}" == "${value}" ]; then
		printf "Success: \"%s\" >= \"%s\"\n" "${input}" "${value}" >&2
	else
		printf "Expected \"%s\" >= \"%s\"\n" "${input}" "${value}" >&2
		return 1
	fi
}

check() {
	local want got

	want=${1}

	got=
	while read -r d; do
		if [ -z "${got}" ]; then
			got="${d}"
		else
			got="${got}\n${d}"
		fi
	done

	OUT=$(echo "${got}" | grep -E "${want}" || echo "(NOT FOUND)")
	if [[ ${OUT} == "(NOT FOUND)" ]]; then
		echo "" >&2
		# shellcheck disable=SC2059
		printf "$(red \"Expected\"): ${want}\n" >&2
		# shellcheck disable=SC2059
		printf "$(red \"Received\"): ${got}\n" >&2
		echo "" >&2
		return 1
	fi
}
