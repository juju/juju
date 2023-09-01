# Retry a command until it succeeds (returns exit code 0). The retry strategy
# is a simple: retry after a fixed delay.
#   usage: retry <command> [max_retries] [delay]
#
# Arguments:
#   - command (required): the name of the command to retry. Usually a Bash
#     function. The function should 'return 1' instead of 'exit 1', as calling
#     'exit' will terminate the whole test.
#   - max_retries (optional): the number of times to retry. Defaults to 5.
#   - delay (optional): the amount of time to sleep after an attempt. Defaults
#     to 5 seconds.
retry() {
	local command=${1}
	local max_retries=${2:-5} # default: 5 retries
	local delay=${3:-5} # default delay: 5s

	local attempt=1
	while true; do
		echo "$command: attempt $attempt"
		$command && break  # if the command succeeds, break the loop

		if [[ $attempt -ge $max_retries ]]; then
			echo "$command failed after $max_retries retries"
			return 1
		fi

		attempt=$((attempt+1))
		sleep $delay
	done
}