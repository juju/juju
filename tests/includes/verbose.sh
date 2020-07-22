set_verbosity() {
    # There are three levels of verbosity, both 1 and 2 will fail on any error,
    # the difference is that 2 will also turn juju debug statements on, but not
    # the shell debug statements. Turning to 3, turns everything on, in theory
    # we could also turn on trace statements for juju.
    case "${VERBOSE}" in
    1)
        set -eu
        ;;
    2)
        set -eu
        ;;
    *)
        echo "Unexpected verbose level" >&2
        exit 1
        ;;
    esac
}
