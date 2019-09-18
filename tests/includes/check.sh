check_dependencies() {
    local dep missing
    missing=""

    for dep in "$@"; do
        if ! which "$dep" >/dev/null 2>&1; then
            [ "$missing" ] && missing="$missing $dep" || missing="$dep"
        fi
    done

    if [ "$missing" ]; then
        echo "Missing dependencies: $missing" >&2
        echo ""
        exit 1
    fi
}
