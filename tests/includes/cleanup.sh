add_clean_func() {
    local name

    name=${1}

    echo "${name}" > "${TEST_DIR}/cleanup"
}

# cleanup_funcs attempts to clean up with functions.
cleanup_funcs() {
    if [ -f "${TEST_DIR}/cleanup" ]; then
        while read -r CMD; do
            echo "====> Running clean up func: ${CMD}"
            $CMD
            echo "====> Finished cleaning up func: ${CMD}"
        done < "${TEST_DIR}/cleanup"
    fi
}
