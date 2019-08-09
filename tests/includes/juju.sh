destroy() {
    local name

    name=${1}
    shift

    echo "==> Destroying juju ${name}"
    echo "${name}" | xargs -I % juju destroy-controller --destroy-all-models -y %
    echo "==> Destroyed juju ${name}"
}

cleanup_jujus() {
    echo "==> Cleaning up jujus"

    OUT=$(juju controllers --format=json | jq '.controllers | keys | .[]')
    for NAME in $(echo "${OUT}" | tr " " "\n"); do
        destroy "${NAME}"
    done
}
