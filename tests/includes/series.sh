# base_to_series converts a base to a series.
# eg "ubuntu@22.04" -> "jammy"
# ```
# base_to_series <base>
# ```
base_to_series() {
	local base base_parts series os channel

	base=${1}
	IFS='@' read -ra base_parts <<<"$base"
	os=${base_parts[0]}
	channel=${base_parts[1]:-}
	case "${os}" in
	"ubuntu")
		case "${channel}" in
		"20.04")
			series="focal"
			;;
		"22.04")
			series="jammy"
			;;
		esac
		;;
	"centos")
		series=${os}${channel}
		;;
	*)
		if [[ -z ${channel} ]]; then
			series=${os}
		else
			series=""
		fi
		;;
	esac
	echo "${series}"
}
