supports_colors() {
	if [[ -z ${TERM} ]] || [[ ${TERM} == "" ]] || [[ ${TERM} == "dumb" ]] || [[ ${TERM} == "unknown" ]]; then
		echo "NO"
		return
	fi
	if which tput >/dev/null 2>&1; then
		# shellcheck disable=SC2046
		if [[ $(tput colors 2>/dev/null || echo 0) -gt 1 ]]; then
			echo "YES"
			return
		fi
	fi
	echo "NO"
}

red() {
	if [[ "$(supports_colors)" == "YES" ]]; then
		tput sgr0
		echo "$(tput setaf 1)${1}$(tput sgr0)"
		return
	fi
	echo "${1}"
}

green() {
	if [[ "$(supports_colors)" == "YES" ]]; then
		tput sgr0
		echo "$(tput setaf 2)${1}$(tput sgr0)"
		return
	fi
	echo "${1}"
}
