set_verbosity() {
    case "${VERBOSE}" in
    1)
        set -e
        ;;
    2)
        set -eux
        ;;
    *)
        echo "Unexpected verbose level" >&2
        exit 1
        ;;
    esac
}
