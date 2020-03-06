check_dependencies() {
    local dep missing
    missing=""

    for dep in "$@"; do
        if ! which "$dep" >/dev/null 2>&1; then
            [ "$missing" ] && missing="$missing $dep" || missing="$dep"
        fi
    done

    if [ "$missing" ]; then
        echo "Missing dependencies: $missing" >&2
        echo ""
        exit 1
    fi
}

check_not_contains() {
    local input value chk

    input=${1}
    shift

    value=${1}
    shift

    chk=$(echo "${input}" | grep "${value}" || true)
    if [ -n "${chk}" ]; then
        printf "Unexpected \"${value}\" found\n\n%s\n" "${input}" >&2
        exit 1
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
    if [ -z "${chk}" ]; then
        printf "Expected \"${value}\" not found\n\n%s\n" "${input}" >&2
        exit 1
    else
        echo "Success: \"${value}\" found" >&2
    fi
}

check() {
    local want got

    want=${1}

    got=
    while read -r d; do
        got="${got}\n${d}"
    done

    OUT=$(echo "${got}" | grep -E "${want}" || true)
    if [ -z "${OUT}" ]; then
        echo "" >&2
        # shellcheck disable=SC2059
        printf "$(red \"Expected\"): ${want}\n" >&2
        # shellcheck disable=SC2059
        printf "$(red \"Recieved\"): ${got}\n" >&2
        echo "" >&2
        exit 1
    fi
}