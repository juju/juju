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

# _secret_token_rbac_names returns a stable comma-separated list of token RBAC
# resource names for a consumer.
_secret_token_rbac_names() {
	local namespace=${1}
	local consumer=${2}
	local kind=${3}

	kubectl -n "$namespace" get "$kind" -l "secrets.juju.is/consumer=${consumer}" -o json |
		yq -r '.items[].metadata.name | select(test("^juju-secret-consumer-"))' |
		sort |
		paste -sd, -
}

# secret_token_rbac_snapshot returns serviceaccount/role/rolebinding names for
# strict before/after reuse checks.
secret_token_rbac_snapshot() {
	local namespace=${1}
	local consumer=${2}

	local serviceaccounts
	local roles
	local rolebindings

	serviceaccounts=$(_secret_token_rbac_names "$namespace" "$consumer" serviceaccounts)
	roles=$(_secret_token_rbac_names "$namespace" "$consumer" roles)
	rolebindings=$(_secret_token_rbac_names "$namespace" "$consumer" rolebindings)

	printf 'serviceaccounts=%s\nroles=%s\nrolebindings=%s\n' "$serviceaccounts" "$roles" "$rolebindings"
}

# _secret_token_rbac_csv_count returns the number of items in a comma-separated
# list; empty input returns zero.
_secret_token_rbac_csv_count() {
	local csv=${1}
	if [[ -z $csv ]]; then
		echo 0
		return
	fi
	awk -F',' '{print NF}' <<<"$csv"
}

# secret_token_rbac_assert_singleton asserts there is exactly one token RBAC
# tuple and all names match.
secret_token_rbac_assert_singleton() {
	local snapshot=${1}
	local context=${2}

	local serviceaccounts
	local roles
	local rolebindings

	serviceaccounts=$(echo "$snapshot" | awk -F= '/^serviceaccounts=/{print $2}')
	roles=$(echo "$snapshot" | awk -F= '/^roles=/{print $2}')
	rolebindings=$(echo "$snapshot" | awk -F= '/^rolebindings=/{print $2}')

	if [[ $(_secret_token_rbac_csv_count "$serviceaccounts") -ne 1 ||
	$(_secret_token_rbac_csv_count "$roles") -ne 1 ||
	$(_secret_token_rbac_csv_count "$rolebindings") -ne 1 ]]; then
		echo "Failed: expected exactly one token serviceaccount/role/rolebinding for $context."
		echo "$snapshot"
		return 1
	fi
}

# secret_token_rbac_assert_reused asserts RBAC snapshot is unchanged across
# repeated secret-get operations.
secret_token_rbac_assert_reused() {
	local before_snapshot=${1}
	local after_snapshot=${2}
	local context=${3}

	if [[ $before_snapshot != "$after_snapshot" ]]; then
		echo "Failed: token RBAC resources changed while validating reuse for $context."
		echo "Before:"
		echo "$before_snapshot"
		echo "After:"
		echo "$after_snapshot"
		return 1
	fi
}
