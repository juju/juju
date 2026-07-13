# check_num_secret_revisions checks the number of secret revisions for a given
# secret is equal to the expected number. As secret pruning is not instant (it
# can take a couple of seconds), we do this in a retry loop to improve the
# resilience of this check.
#
#   usage: check_num_secret_revisions <secret_uri> <secret_short_uri> <expected_num_revisions>
check_num_secret_revisions() {
	local secret_uri=${1}
	local secret_short_uri=${2}
	local expected_num_revisions=${3}

	local max_retries=5
	local delay=5

	local attempt=1
	while true; do
		echo "Checking secret revisions for $secret_short_uri: attempt $attempt"
		check_contains "$(juju --show-log show-secret "$secret_uri" --revisions |
			yq ".${secret_short_uri}.revisions | length")" "$expected_num_revisions" &&
			break # if the command succeeds, break the loop

		if [[ $attempt -ge $max_retries ]]; then
			echo "Checking secret revisions failed after $max_retries retries"
			return 1
		fi

		attempt=$((attempt + 1))
		sleep $delay
	done
}

# check_num_secrets checks the total number of user secrets equals the
# expected count. Since remove-secret is now asynchronous (scheduled via
# a removal job), we retry to give the removal worker time to process it.
#
#   usage: check_num_secrets <expected_count>
check_num_secrets() {
	local expected=${1}

	local max_retries=5
	local delay=5

	local attempt=1
	while true; do
		local actual
		actual=$(juju --show-log secrets --format yaml | yq length)
		if [ "$actual" -eq "$expected" ] 2>/dev/null; then
			echo "Success: $expected secret(s) found"
			return 0
		fi
		echo "Checking total secrets: attempt $attempt - expected $expected, got $actual"

		if [[ $attempt -ge $max_retries ]]; then
			echo "Checking total secrets failed after $max_retries retries"
			return 1
		fi

		attempt=$((attempt + 1))
		sleep $delay
	done
}
