# juju_version will return only the version and not the architecture/substrait
# of the juju version.
# This will use any juju on $PATH
juju_version() {
    version=$(juju version | cut -f1 -d '-')
    echo "${version}"
}

# ensure will check if there is a bootstrapped controller that it can take
# advantage of, failing that it will bootstrap a new controller for you.
#
# ```
# ensure <model name> <file to output logs>
# ```
ensure() {
    local model output

    model=${1}
    shift

    output=${1}
    shift

    export BOOTSTRAP_REUSE="true"
    bootstrap "${model}" "${output}"
}

# bootstrap will attempt to bootstrap a controller on the correct provider.
# It will check if there is an existing controller with the same name and bail,
# if there is.
#
# The name of the controller is randomised, but the model name is used to
# override the default model name for that controller. That way we have a
# unqiue namespaced models instead of the "default" model name.
# This helps with providing encapsulated tests without having to bootstrap a
# controller for every test in a suite.
#
# The stdout of the file can be piped to an optional output file.
#
# ```
# bootstrap <model name> <file to output logs>
# ```
bootstrap() {
    local provider name output model bootstrapped_name
    case "${BOOTSTRAP_PROVIDER:-}" in
        "aws")
            provider="aws"
            ;;
        "lxd")
            provider="lxd"
            ;;
        "localhost")
            provider="lxd"
            ;;
        "manual")
            manual_name=${1}
            shift

            provider="${manual_name}"
            ;;
        "microk8s")
          provider="microk8s"
          ;;
        *)
            echo "Unexpected bootstrap provider (${BOOTSTRAP_PROVIDER})."
            exit 1
    esac

    model=${1}
    shift

    output=${1}
    shift

    rnd=$(head /dev/urandom | tr -dc a-z0-9 | head -c 8; echo '')
    name="ctrl-${rnd}"

    if [ ! -f "${TEST_DIR}/jujus" ]; then
        touch "${TEST_DIR}/jujus"
    fi
    bootstrapped_name=$(grep "." "${TEST_DIR}/jujus" | tail -n 1)
    if [ -z "${bootstrapped_name}" ]; then
        # shellcheck disable=SC2236
        if [ ! -z "${BOOTSTRAP_REUSE_LOCAL}" ]; then
            bootstrapped_name="${BOOTSTRAP_REUSE_LOCAL}"
            export BOOTSTRAP_REUSE="true"
        else
            # No bootstrapped juju found, unset the the variable.
            echo "====> Unable to reuse bootstrapped juju"
            export BOOTSTRAP_REUSE="false"
        fi
    fi
    if [ "${BOOTSTRAP_REUSE}" = "true" ]; then
        OUT=$(juju show-machine -m "${bootstrapped_name}":controller --format=json | jq -r ".machines | .[] | .series")
        if [ -n "${OUT}" ]; then
            OUT=$(echo "${OUT}" | grep -oh "${BOOTSTRAP_SERIES}" || true)
            if [ "${OUT}" != "${BOOTSTRAP_SERIES}" ]; then
                echo "====> Unable to reuse bootstrapped juju"
                export BOOTSTRAP_REUSE="false"
            fi
        fi
    fi

    version=$(juju_version)

    START_TIME=$(date +%s)
    if [ "${BOOTSTRAP_REUSE}" = "true" ]; then
        echo "====> Reusing bootstrapped juju ($(green "${version}:${provider}"))"

        OUT=$(juju models -c "${bootstrapped_name}" --format=json 2>/dev/null | jq '.models[] | .["short-name"]' | grep "${model}" || true)
        if [ -n "${OUT}" ]; then
            echo "${model} already exists. Use the following to clean up the environment:"
            echo "    juju switch ${bootstrapped_name}"
            echo "    juju destroy-model --force -y ${model}"
            exit 1
        fi

        add_model "${model}" "${provider}" "${bootstrapped_name}"
        name="${bootstrapped_name}"
    else
        echo "====> Bootstrapping juju ($(green "${version}:${provider}"))"
        juju_bootstrap "${provider}" "${name}" "${model}" "${output}"
    fi
    END_TIME=$(date +%s)

    echo "====> Bootstrapped juju ($((END_TIME-START_TIME))s)"

    export BOOTSTRAPPED_JUJU_CTRL_NAME="${name}"
}

# add_model is used to add a model for tracking. This is for internal use only
# and shouldn't be used by any of the tests directly.
add_model() {
    local model provider controller

    model=${1}
    provider=${2}
    controller=${3}

    OUT=$(juju controllers --format=json | jq '.controllers | .["${bootstrapped_name}"] | .cloud' | grep "${provider}" || true)
    if [ -n "${OUT}" ]; then
        juju add-model -c "${controller}" "${model}" "${provider}"
    else
        juju add-model -c "${controller}" "${model}"
    fi
    echo "${model}" >> "${TEST_DIR}/models"
}

# juju_bootstrap is used to bootstrap a model for tracking. This is for internal
# use only and shouldn't be used by any of the tests directly.
juju_bootstrap() {
    local provider name model output

    provider=${1}
    shift

    name=${1}
    shift

    model=${1}
    shift

    output=${1}
    shift

    series=
    case "${BOOTSTRAP_SERIES}" in
    "${CURRENT_LTS}")
        series="--bootstrap-series=${BOOTSTRAP_SERIES} --config image-stream=daily --force"
        ;;
    "")
        ;;
    *)
        series="--bootstrap-series=${BOOTSTRAP_SERIES}"
    esac

    debug="false"
    if [ "${VERBOSE}" -gt 1 ]; then
        debug="true"
    fi


    if [ -n "${output}" ]; then
        # When double quotes are added to ${series}, the juju bootstrap
        # command looks correct, and works outside of the harness, but
        # does not run, goes directly to cleanup.
        #shellcheck disable=SC2086
        juju bootstrap ${series} --debug="${debug}" "${provider}" "${name}" -d "${model}" "$@" > "${output}" 2>&1
    else
        #shellcheck disable=SC2086
        juju bootstrap ${series} --debug="${debug}" "${provider}" "${name}" -d "${model}" "$@"
    fi
    echo "${name}" >> "${TEST_DIR}/jujus"
}

# destroy_model takes a model name and destroys a model. It first checks if the
# model is found before attempting to do so.
#
# ```
# destroy_model <model name>
# ```
destroy_model() {
    local name

    name=${1}
    shift

    # shellcheck disable=SC2034
    OUT=$(juju models --format=json | jq '.models | .[] | .["short-name"]' | grep "${name}" || true)
    # shellcheck disable=SC2181
    if [ -z "${OUT}" ]; then
        return
    fi

    output="${TEST_DIR}/${name}-destroy.txt"

    echo "====> Destroying juju model ${name}"
    echo "${name}" | xargs -I % juju destroy-model --force -y % >"${output}" 2>&1
    CHK=$(cat "${output}" | grep -i "ERROR" || true)
    if [ -n "${CHK}" ]; then
        printf "\\nFound some issues\\n"
        cat "${output}"
        exit 1
    fi
    echo "====> Destroyed juju model ${name}"
}

# destroy_controller takes a controller name and destroys the controller. It
# also destroys all the models at the same time.
#
# ```
# destroy_controller <controller name>
# ```
destroy_controller() {
    local name

    name=${1}
    shift

    # shellcheck disable=SC2034
    OUT=$(juju controllers --format=json | jq '.controllers | keys[]' | grep "${name}" || true)
    # shellcheck disable=SC2181
    if [ -z "${OUT}" ]; then
        OUT=$(juju models --format=json | jq -r ".models | .[] | .[\"short-name\"]" | grep "^${name}$" || true)
        if [ -z "${OUT}" ]; then
            echo "====> ERROR Destroy controller/model. Unable to locate $(red "${name}")"
            exit 1
        fi
        echo "====> Destroying model ($(green "${name}"))"

        output="${TEST_DIR}/${name}-destroy-model.txt"
        echo "${name}" | xargs -I % juju destroy-model --force -y % >"${output}" 2>&1

        echo "====> Destroyed model ($(green "${name}"))"
        return
    fi

    set +e

    echo "====> Introspection gathering"
    introspect_controller "${name}" || true
    echo "====> Introspection gathered"

    # Unfortunately having any offers on a model, leads to failure to clean
    # up a controller.
    # See discussion under https://bugs.launchpad.net/juju/+bug/1830292.
    echo "====> Removing offers"
    remove_controller_offers "${name}"
    echo "====> Removed offers"

    set_verbosity

    output="${TEST_DIR}/${name}-destroy-controller.txt"

    echo "====> Destroying juju ($(green "${name}"))"
    echo "${name}" | xargs -I % juju destroy-controller --destroy-all-models -y % >"${output}" 2>&1

    set +e
    CHK=$(cat "${output}" | grep -i "ERROR" || true)
    if [ -n "${CHK}" ]; then
        printf "\\nFound some issues\\n"
        cat "${output}"
        exit 1
    fi
    set_verbosity

    sed -i "/^${name}$/d" "${TEST_DIR}/jujus"
    echo "====> Destroyed juju ($(green "${name}"))"
}

# cleanup_jujus is used to destroy all the known controllers the test suite
# knows about. This is for internal use only and shouldn't be used by any of the
# tests directly.
cleanup_jujus() {
    if [ -f "${TEST_DIR}/jujus" ]; then
        echo "====> Cleaning up jujus"

        while read -r juju_name; do
            destroy_controller "${juju_name}"
        done < "${TEST_DIR}/jujus"
        rm -f "${TEST_DIR}/jujus" || true
    fi
    echo "====> Completed cleaning up jujus"
}

introspect_controller() {
    local name

    name=${1}

    idents=$(juju machines -m "${name}:controller" --format=json | jq ".machines | keys | .[]")
    if [ -z "${idents}" ]; then
        return
    fi

    echo "${idents}" | xargs -I % juju ssh -m "${name}:controller" % bash -lc "juju_engine_report" > "${TEST_DIR}/${name}-juju_engine_reports.txt"
    echo "${idents}" | xargs -I % juju ssh -m "${name}:controller" % bash -lc "juju_goroutines" > "${TEST_DIR}/${name}-juju_goroutines.txt"
}

remove_controller_offers() {
    local name

    name=${1}

    OUT=$(juju models -c "${name}" --format=json | jq -r ".[\"models\"] | .[] | select(.[\"is-controller\"] == false) | .name" || true)
    if [ -n "${OUT}" ]; then
        echo "${OUT}" | while read -r model; do
            OUT=$(juju offers -m "${name}:${model}" --format=json | jq -r ".[] | .[\"offer-url\"]" || true)
            echo "${OUT}" | while read -r offer; do
                if [ -n "${offer}" ]; then
                    juju remove-offer --force -y -c "${name}" "${offer}"
                    echo "${offer}" >> "${TEST_DIR}/${name}-juju_removed_offers.txt"
                fi
            done
        done
    fi
}
