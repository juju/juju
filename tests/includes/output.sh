OUTPUT() {
    local output

    output=${1}
    shift

    if [ -z "${output}" ] || [ "${VERBOSE}" -gt 1 ]; then
        echo
    fi

    while read data; do
        if [ -z "${output}" ]; then
            echo "${data}"
        elif [ "${VERBOSE}" -le 1 ]; then
            echo "${data}" >> "${output}"
        elif echo "${data}" | grep -q "^\s*$"; then
            echo "${data}" | tee -a "${output}"
        else
            echo "${data}" | tee -a "${output}" | sed 's/^/    | /g'
        fi
    done

    if [ -z "${output}" ] || [ "${VERBOSE}" -gt 1 ]; then
        echo
    fi
}