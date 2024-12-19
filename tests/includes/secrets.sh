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
