check_deps() {
    echo "Phase 0: Checking requirements."
    has_deps=1
    which git || has_deps=0
    which go || has_deps=0
    if [[ $has_deps == 0 ]]; then
        echo "Install git and golang."
        exit 2
    fi
}


test $# -ge 1 ||  usage
check_deps
