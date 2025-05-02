run_schema() {
	TMP_ORIG=$(mktemp -d /tmp/schema-XXXXX)
	git show "HEAD:apiserver/facades/schema.json" >"${TMP_ORIG}/schema.json"
	CUR_SHA=$(cat "${TMP_ORIG}/schema.json" | shasum -a 1 | awk '{ print $1 }')
	git show "HEAD:apiserver/facades/agent-schema.json" >"${TMP_ORIG}/agent-schema.json"
	CUR_AGENT_SHA=$(cat "${TMP_ORIG}/agent-schema.json" | shasum -a 1 | awk '{ print $1 }')
	TMP=$(mktemp -d /tmp/schema-XXXXX)
	OUT=$(make --no-print-directory SCHEMA_PATH="${TMP}" rebuild-schema 2>&1)
	OUT_CODE=$?
	if [ $OUT_CODE -ne 0 ]; then
		echo ""
		echo "$(red 'Found some issues:')"
		echo "${OUT}"
		exit 1
	fi
	# shellcheck disable=SC2002
	NEW_SHA=$(cat "${TMP}/schema.json" | shasum -a 1 | awk '{ print $1 }')
	# shellcheck disable=SC2002
	NEW_AGENT_SHA=$(cat "${TMP}/agent-schema.json" | shasum -a 1 | awk '{ print $1 }')

	if [ "${CUR_SHA}" != "${NEW_SHA}" ]; then
		(echo >&2 "\\nError: client facades schema is not in sync. Run 'make rebuild-schema' and commit source.")
		(diff >&2 "${TMP_ORIG}/schema.json" "${TMP}/schema.json")
		exit 1
	fi
	if [ "${CUR_AGENT_SHA}" != "${NEW_AGENT_SHA}" ]; then
		(echo >&2 "\\nError: agent facades schema is not in sync. Run 'make rebuild-schema' and commit source.")
		(diff >&2 "${TMP_ORIG}/agent-schema.json" "${TMP}/agent-schema.json")
		exit 1
	fi
}

test_schema() {
	if [ "$(skip 'test_schema')" ]; then
		echo "==> TEST SKIPPED: static schema analysis"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		# Check for schema changes and ensure they've been committed
		run_linter "run_schema"
	)
}
