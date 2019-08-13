run_schema() {
    CUR_SHA=$(git show HEAD:apiserver/facades/schema.json | shasum -a 1 | awk '{ print $1 }')
    TMP=$(mktemp /tmp/schema-XXXXX)
    OUT=$(make SCHEMA_PATH="${TMP}" rebuild-schema 2>&1)
    if [ "${OUT}" != "Generating facade schema..." ]; then
        printf "\\Found some issues:"
        echo "${OUT}"
        exit 1
    fi
    # shellcheck disable=SC2002
    NEW_SHA=$(cat "${TMP}" | shasum -a 1 | awk '{ print $1 }')

    if [ "${CUR_SHA}" != "${NEW_SHA}" ]; then
        (>&2 echo "\\nError: facades schema is not in sync. Run 'make rebuild-schema' and commit source.")
        exit 1
    fi
}

test_schema() {
    if [ -n "${SKIP_STATIC_SCHEMA:-}" ]; then
        echo "==> SKIP: Asked to skip static schema analysis"
        return
    fi

    (
        set -e

        cd ../

        # Check for schema changes and ensure they've been committed
        run "schema"
    )
}
