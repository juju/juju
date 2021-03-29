OUTPUT() {
	local output

	output=${1}
	shift

	if [[ -z "${output}" ]] || [[ "${VERBOSE}" -gt 1 ]]; then
		echo
	fi

	# shellcheck disable=SC2162
	while read data; do
		# If there is no output, just dump straight to stdout.
		if [[ -z "${output}" ]]; then
			echo "${data}"
		# If there is an output, but we're not in verbose mode, just append to
		# the output.
		elif [[ "${VERBOSE}" -le 1 ]]; then
			echo "${data}" >>"${output}"
		# If we are in verbose mode, but we're an empty line, send to stdout
		# and also tee it to the output.
		elif echo "${data}" | grep -q "^\s*$"; then
			echo "${data}" | tee -a "${output}"
		# Finally, we have content and we're in verbose mode. Send the data to
		# the output and then format it for stdout.
		else
			echo "${data}" | tee -a "${output}" | sed 's/^/    | /g'
		fi
	done

	if [[ -z "${output}" ]] || [[ "${VERBOSE}" -gt 1 ]]; then
		echo
	fi
}
