set_verbosity() {
    # There are three levels of verbosity, both 1 and 2 will fail on any error,
    # the difference is that 2 will also turn juju debug statements on, but not
    # the shell debug statements. Turning to 3, turns everything on, in theory
    # we could also turn on trace statements for juju.
    case "${VERBOSE}" in
    1)
        set -e
        ;;
    2)
        set -e
        ;;
    3)
        set -eux
        ;;
    *)
        echo "Unexpected verbose level" >&2
        exit 1
        ;;
    esac
}

set_test_verbosity() {
    # There are three levels of verbosity, both 1 and 2 will fail on any error,
    # the difference is that 2 will also turn convoy debug statements on, but not
    # the shell debug statements. Turning to 3, turns everything on, in theory
    # we could also turn on trace statements for convoy.
    case "${TEST_VERBOSE}" in
    1)
        set -e
        ;;
    2)
        set -e
        ;;
    3)
        set -eux
        ;;
    *)
        echo "Unexpected verbose level" >&2
        exit 1
        ;;
    esac
}
