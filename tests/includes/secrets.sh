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

# count_secret_changed_hooks returns the number of times <unit> in <model>
# has run the secret-changed hook, as recorded in the unit's status
# history. With <secret_id>, only hooks for that secret are counted; the
# match is on the bare ID because a consumer may render the owner's URI in
# a different form.
#
#   usage: count_secret_changed_hooks <model> <unit> [<secret_id>]
count_secret_changed_hooks() {
	local model=${1} unit=${2} secret_id=${3:-} out count

	out=$(juju show-status-log -m "${model}" "${unit}" --format json -n 500 2>/dev/null) || true
	count=$(printf '%s\n' "${out}" | SECRET_ID="${secret_id}" yq -r \
		'[.[] | select(.message != null) | select(.message | contains("running secret-changed hook")) | select(.message | contains(strenv(SECRET_ID)))] | length' \
		2>/dev/null) || true
	echo "${count:-0}"
}

# wait_for_hook_count_stable waits until the secret-changed hook count of a
# unit stops increasing, so that a baseline count is reliable. Registration
# of a consumed secret can fire a secret-changed hook asynchronously.
#
#   usage: wait_for_hook_count_stable <model> <unit> [<secret_id>]
wait_for_hook_count_stable() {
	local model=${1} unit=${2} secret_id=${3:-} previous=-1 current attempt=0

	while true; do
		current=$(count_secret_changed_hooks "${model}" "${unit}" "${secret_id}")
		if [[ ${current} -eq ${previous} ]]; then
			echo "[+] $(green 'secret-changed hook count is stable for') $(green "${unit}${secret_id:+ (${secret_id})}"): ${current}"
			return 0
		fi
		echo "[+] (attempt ${attempt}) polling secret-changed hook count for ${model} ${unit}${secret_id:+ (${secret_id})} => ${current} (previous ${previous})"
		juju show-status-log -m "${model}" "${unit}" -n 10 2>&1 | sed 's/^/    | /g'
		previous=${current}
		attempt=$((attempt + 1))
		if [[ ${attempt} -ge 6 ]]; then
			echo "[-] $(red "secret-changed hook count for ${model} ${unit}${secret_id:+ (${secret_id})} did not stabilise (${current})")"
			exit 1
		fi
		sleep "${SHORT_TIMEOUT}"
	done
}

# wait_for_secret_changed_hooks waits until <unit> in <model> has run at
# least <expected> secret-changed hooks (for <secret_id> when given). On
# timeout the model's debug-log is dumped.
#
#   usage: wait_for_secret_changed_hooks <model> <unit> <expected> [<secret_id>]
wait_for_secret_changed_hooks() {
	local model=${1} unit=${2} expected=${3} secret_id=${4:-} current attempt=0

	while true; do
		current=$(count_secret_changed_hooks "${model}" "${unit}" "${secret_id}")
		if [[ ${current} -ge ${expected} ]]; then
			echo "[+] $(green 'Completed polling secret-changed hooks for') $(green "${unit}${secret_id:+ (${secret_id})}"): ${current}"
			return 0
		fi
		echo "[+] (attempt ${attempt}) polling secret-changed hooks for ${model} ${unit}${secret_id:+ (${secret_id})} => ${current} (want >= ${expected})"
		juju show-status-log -m "${model}" "${unit}" -n 10 2>&1 | sed 's/^/    | /g'
		attempt=$((attempt + 1))
		if [[ ${attempt} -ge 40 ]]; then
			echo "[-] $(red "timed out waiting for ${expected} secret-changed hook(s) on ${model} ${unit}${secret_id:+ (${secret_id})}, got ${current}")"
			echo "    (model) juju debug-log output"
			juju debug-log -m "${model}" --replay --no-tail 2>&1 | sed 's/^/    | /g'
			exit 1
		fi
		sleep "${SHORT_TIMEOUT}"
	done
}
