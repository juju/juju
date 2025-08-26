#!/usr/bin/env -S bash -e
# juju_version will return only the version and not the architecture/substrate
# of the juju version. If JUJU_VERSION is defined in CI this value will be used
# otherwise we interrogate the juju binary on path.
juju_version() {
	# Match only major, minor, and patch or tag + build number
	if [ -n "${JUJU_VERSION:-}" ]; then
		version=${JUJU_VERSION}
	else
		version=$(juju version | grep -oE '^[[:digit:]]+\.[[:digit:]]+(\.[[:digit:]]+|-\w+){1}(\.[[:digit:]]+)?')
	fi
	echo "${version}"
}

jujud_version() {
	version=$(jujud version)

	# shellcheck disable=SC2116
	version=$(echo "${version%-*}")
	# shellcheck disable=SC2116
	version=$(echo "${version%-*}")

	echo "${version}"
}

# ensure will bootstrap a controller if there are none available. It will then
# add the named model to the controller. If the model already exists it will
# fail.
#
# TODO (aflynn50 2024-02-15): Implement the expected behaviour:
# ensure should check if there is a bootstrapped controller that it can take
# advantage of, failing that it should bootstrap a new controller for you.
# It should then check if the expected model present on the controller and add it
# if not.
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

# bootstrap creates a new controller with a random name and then adds the
# specified model to it.
#
# If BOOTSTRAP_REUSE is set it should reuse the
# controller named in BOOTSTRAP_REUSE_LOCAL, if this is blank it will use any
# existing test controller.
#
# If BOOTSTRAP_PROVIDER is set to "manual" then <cloud-name> should be provided
# as the first arg.
#
# The name of the controller is randomised, but the model name is used to
# override the default model name for that controller. That way we have a
# unique namespaced models instead of the "default" model name.
# This helps with providing encapsulated tests without having to bootstrap a
# controller for every test in a suite.
#
# The stdout is piped to the specified log file.
# ```
# bootstrap <model name> <file to output logs>
# ```
bootstrap() {
	local cloud name output model bootstrapped_name
	# Handle provider aliases.
	case "${BOOTSTRAP_PROVIDER:-}" in
	"aws")
		BOOTSTRAP_PROVIDER="ec2"
		;;
	"google")
		BOOTSTRAP_PROVIDER="gce"
		;;
	esac

	case "${BOOTSTRAP_PROVIDER:-}" in
	"ec2")
		cloud="aws"
		;;
	"gce")
		cloud="google"
		;;
	"azure")
		cloud="azure"
		;;
	"microk8s")
		cloud="microk8s"
		;;
	"lxd")
		cloud="${BOOTSTRAP_CLOUD:-localhost}"
		;;
	"vsphere" | "openstack" | "maas")
		cloud="${BOOTSTRAP_CLOUD}"
		if [[ -z ${cloud} ]]; then
			echo "must specify cloud to bootstrap for provider ${BOOTSTRAP_PROVIDER}"
			exit 1
		fi
		;;
	"k8s")
		cloud="${BOOTSTRAP_CLOUD:-microk8s}"
		;;
	"manual")
		manual_name=${1}
		shift

		cloud="${manual_name}"
		;;
	*)
		echo "Unexpected bootstrap provider (${BOOTSTRAP_PROVIDER})."
		exit 1
		;;
	esac

	model=${1}
	shift

	output=${1}
	shift

	rnd=$(rnd_str)
	name="ctrl-${rnd}"

	if [[ ! -f "${TEST_DIR}/jujus" ]]; then
		touch "${TEST_DIR}/jujus"
	fi
	bootstrapped_name=$({ grep "." "${TEST_DIR}/jujus" || echo ""; } | tail -n 1)
	if [[ -z ${bootstrapped_name} ]]; then
		# shellcheck disable=SC2236
		if [[ -n ${BOOTSTRAP_REUSE_LOCAL} ]]; then
			bootstrapped_name="${BOOTSTRAP_REUSE_LOCAL}"
			export BOOTSTRAP_REUSE="true"
		else
			# No bootstrapped juju found, unset the the variable.
			echo "====> Unable to reuse bootstrapped juju"
			export BOOTSTRAP_REUSE="false"
		fi
	fi
	if [[ ${BOOTSTRAP_REUSE} == "true" && ${BOOTSTRAP_PROVIDER} != "k8s" ]]; then
		# juju show-machine not supported with k8s controllers
		OUT=$(juju show-machine -m "${bootstrapped_name}":controller --format=json | jq -r '.machines | .[] | .base | (.name + "@" + .channel)')
		if [[ -n ${OUT} ]]; then
			OUT=$(echo "${OUT}" | grep -oh "${BOOTSTRAP_BASE}" || true)
			if [[ ${OUT} != "${BOOTSTRAP_BASE}" ]]; then
				echo "====> Unable to reuse bootstrapped juju"
				export BOOTSTRAP_REUSE="false"
			fi
		fi
	fi

	version=$(juju_version)

	START_TIME=$(date +%s)
	if [[ ${BOOTSTRAP_REUSE} == "true" ]]; then
		echo "====> Reusing bootstrapped juju ($(green "${version}:${cloud}"))"

		OUT=$(juju models -c "${bootstrapped_name}" --format=json 2>/dev/null | jq -r ".models[] | .[\"short-name\"] | select(. == \"${model}\")" || true)
		if [[ -n ${OUT} ]]; then
			echo "${model} already exists. Use the following to clean up the environment:"
			echo "    juju switch ${bootstrapped_name}"
			echo "    juju destroy-model --no-prompt ${model}"
			exit 1
		fi

		juju_add_model "${model}" "${cloud}" "${bootstrapped_name}" "${output}"
		name="${bootstrapped_name}"
		BOOTSTRAPPED_CLOUD=$(juju show-model controller --format json | jq -r '.[] | .cloud')
		export BOOTSTRAPPED_CLOUD
		BOOTSTRAPPED_CLOUD_REGION=$(juju show-model controller --format json | jq -r '.[] | (.cloud + "/" + .region)')
		export BOOTSTRAPPED_CLOUD_REGION
	else
		local cloud_region
		if [[ -n ${BOOTSTRAP_REGION:-} ]]; then
			cloud_region="${cloud}/${BOOTSTRAP_REGION}"
		else
			cloud_region="${cloud}"
		fi
		echo "====> Bootstrapping juju ($(green "${version}:${cloud_region}"))"
		juju_bootstrap "${cloud_region}" "${name}" "${model}" "${output}"
		export BOOTSTRAPPED_CLOUD="${cloud}"
		export BOOTSTRAPPED_CLOUD_REGION="${cloud_region}"
	fi

	END_TIME=$(date +%s)

	echo "====> Bootstrapped juju ($((END_TIME - START_TIME))s)"

	export BOOTSTRAPPED_JUJU_CTRL_NAME="${name}"
}

# juju_add_model is used to add a model for tracking. This is for internal use
# only and shouldn't be used by any of the tests directly.
juju_add_model() {
	local model cloud controller

	model=${1}
	cloud=${2}
	controller=${3}
	output=${4}

	OUT=$(juju controllers --format=json | jq '.controllers | .["${bootstrapped_name}"] | .cloud' | grep "${cloud}" || true)
	if [[ -n ${OUT} ]]; then
		juju add-model --show-log -c "${controller}" "${model}" 2>&1 | OUTPUT "${output}"
	else
		juju add-model --show-log -c "${controller}" "${model}" "${cloud}" 2>&1 | OUTPUT "${output}"
	fi

	post_add_model "${controller}" "${model}"

	echo "${model}" >>"${TEST_DIR}/models"
}

add_model() {
	local model

	model=${1}

	juju add-model --show-log "${model}" 2>&1
	post_add_model "" "${model}"
}

# add_images_for_vsphere is used to add-image with known vSphere template paths for LTS series
# and shouldn't be used by any of the tests directly.
add_images_for_vsphere() {
	juju metadata add-image juju-ci-root/templates/jammy-test-template --base ubuntu@22.04
	juju metadata add-image juju-ci-root/templates/focal-test-template --base ubuntu@20.04
}

# setup_vsphere_simplestreams generates image metadata for use during vSphere bootstrap.  There is
# an assumption made with regards to the template name in the Boston vSphere.  This is for internal
# use only and shouldn't be used by any of the tests directly.
setup_vsphere_simplestreams() {
	local dir base

	dir=${1}
	base=${2:-"ubuntu@22.04"}

	if [[ ! -f ${dir} ]]; then
		mkdir "${dir}" || true
	fi

	cloud_endpoint=$(juju clouds --client --format=json | jq -r ".[\"$BOOTSTRAP_CLOUD\"] | .endpoint")
	# pipe output to test dir, otherwise becomes part of the return value.
	juju metadata generate-image -i juju-ci-root/templates/"${base}"-test-template -r "${BOOTSTRAP_REGION}" -d "${dir}" -u "${cloud_endpoint}" --base "${base}" >>"${TEST_DIR}"/simplestreams 2>&1
}

# juju_bootstrap is used to bootstrap a model for tracking. This is for internal
# use only and shouldn't be used by any of the tests directly.
juju_bootstrap() {
	local cloud_region name model output

	cloud_region=${1}
	shift

	name=${1}
	shift

	model=${1}
	shift

	output=${1}
	shift

	base=
	if [[ ${BOOTSTRAP_PROVIDER} != "k8s" ]]; then
		case "${BOOTSTRAP_BASE}" in
		"${CURRENT_LTS}")
			base="--bootstrap-base=${BOOTSTRAP_BASE} --config image-stream=daily --force"
			;;
		"") ;;

		*)
			base="--bootstrap-base=${BOOTSTRAP_BASE}"
			;;
		esac
	fi

	pre_bootstrap

	command="juju bootstrap ${base} ${cloud_region} ${name} --model-default mode= ${BOOTSTRAP_ADDITIONAL_ARGS}"
	# We run the command through the bash interpreter here to avoid some
	# weird shell expansion behaviour when using --config and other arguments
	# with white space.
	bash -c "exec ${command} $*" 2>&1 | OUTPUT "${output}"
	echo "${name}" >>"${TEST_DIR}/jujus"

	if [[ ${BOOTSTRAP_PROVIDER} != "k8s" ]]; then
		juju switch "${name}:controller"
		wait_for_machine_agent_status "0" "started"
	fi

	# Adding the initial model.
	juju add-model --show-log "${model}" 2>&1

	post_bootstrap "${name}" "${model}"
}

# pre_bootstrap contains setup required before bootstrap specific to providers
# and shouldn't be used by any of the tests directly.
pre_bootstrap() {
	# ensure BOOTSTRAP_ADDITIONAL_ARGS is defined, even if not necessary.
	export BOOTSTRAP_ADDITIONAL_ARGS=${BOOTSTRAP_ADDITIONAL_ARGS:-}
	case "${BOOTSTRAP_PROVIDER:-}" in
	"vsphere")
		echo "====> Creating image simplestream metadata for juju ($(green "${version}:${cloud}"))"

		image_streams_dir=${TEST_DIR}/image-streams
		setup_vsphere_simplestreams "${image_streams_dir}" "${BOOTSTRAP_BASE}"
		export BOOTSTRAP_ADDITIONAL_ARGS="${BOOTSTRAP_ADDITIONAL_ARGS} --metadata-source ${image_streams_dir}"
		;;
	esac

	if [[ ${BUILD_AGENT:-} == true ]]; then
		export BOOTSTRAP_ADDITIONAL_ARGS="${BOOTSTRAP_ADDITIONAL_ARGS:-} --build-agent"
	else
		# In CI tests, both Build and OfficialBuild are set.
		# Juju confuses when it needs to decide the operator image tag to use.
		# So we need to explicitly set the agent version for CI tests.
		if [[ -n ${JUJU_VERSION:-} ]]; then
			version=${JUJU_VERSION}
		else
			version=$(juju_version)
		fi
		export BOOTSTRAP_ADDITIONAL_ARGS="${BOOTSTRAP_ADDITIONAL_ARGS:-} --agent-version=${version}"
	fi

	if [[ -n ${SHORT_GIT_COMMIT:-} ]]; then
		export BOOTSTRAP_ADDITIONAL_ARGS="${BOOTSTRAP_ADDITIONAL_ARGS:-} --model-default agent-metadata-url=https://ci-run-streams.s3.amazonaws.com/builds/build-${SHORT_GIT_COMMIT}/"
		export BOOTSTRAP_ADDITIONAL_ARGS="${BOOTSTRAP_ADDITIONAL_ARGS:-} --model-default agent-stream=testing"
	fi

	if [[ -n ${BOOTSTRAP_ARCH} ]]; then
		export BOOTSTRAP_ADDITIONAL_ARGS="${BOOTSTRAP_ADDITIONAL_ARGS:-} --bootstrap-constraints arch=${BOOTSTRAP_ARCH}"
	fi

	if [[ -n ${OPERATOR_IMAGE_ACCOUNT:-} ]]; then
		export BOOTSTRAP_ADDITIONAL_ARGS="${BOOTSTRAP_ADDITIONAL_ARGS:-} --config caas-image-repo=${OPERATOR_IMAGE_ACCOUNT}"
	fi

	if [[ ${BOOTSTRAP_PROVIDER:-} == "k8s" ]]; then
		if [[ -n ${CONTROLLER_CHARM_PATH_CAAS:-} ]]; then
			export BOOTSTRAP_ADDITIONAL_ARGS="${BOOTSTRAP_ADDITIONAL_ARGS:-} --controller-charm-path=${CONTROLLER_CHARM_PATH_CAAS}"
		fi
		if [[ -n ${CONTROLLER_CHARM_CHANNEL:-} ]]; then
			export BOOTSTRAP_ADDITIONAL_ARGS="${BOOTSTRAP_ADDITIONAL_ARGS:-} --controller-charm-channel=${CONTROLLER_CHARM_CHANNEL}"
		fi
	else
		if [[ -n ${CONTROLLER_CHARM_PATH_IAAS:-} ]]; then
			export BOOTSTRAP_ADDITIONAL_ARGS="${BOOTSTRAP_ADDITIONAL_ARGS:-} --controller-charm-path=${CONTROLLER_CHARM_PATH_IAAS}"
		fi
		case "${BOOTSTRAP_PROVIDER:-}" in
		"ec2" | "gce" | "openstack")
			# Don't use fan unless we really need to.
			if [[ -z ${CONTAINER_NETWORKING_METHOD:-} ]]; then
				CONTAINER_NETWORKING_METHOD="local"
			fi
			;;
		esac
		if [[ -n ${CONTAINER_NETWORKING_METHOD:-} ]]; then
			export BOOTSTRAP_ADDITIONAL_ARGS="${BOOTSTRAP_ADDITIONAL_ARGS:-} --model-default container-networking-method=${CONTAINER_NETWORKING_METHOD}"
		fi
	fi

	echo "====> BOOTSTRAP_ADDITIONAL_ARGS: ${BOOTSTRAP_ADDITIONAL_ARGS}"
}

# post_bootstrap contains actions required after bootstrap specific to providers
# and shouldn't be used by any of the tests directly.  Calls post_add_model
# models are added during bootstrap.
post_bootstrap() {
	local controller model

	controller=${1}
	model=${2}

	# Unset the bootstrap args to reset them for subsequent tests.
	unset BOOTSTRAP_ADDITIONAL_ARGS

	# Setup up log tailing on the controller.
	# shellcheck disable=SC2069
	juju debug-log -m "${controller}:controller" --replay --tail 2>&1 >"${TEST_DIR}/${controller}-controller-debug.log" &
	CMD_PID=$!
	echo "${CMD_PID}" >>"${TEST_DIR}/pids"

	case "${BOOTSTRAP_PROVIDER:-}" in
	"vsphere")
		rm -r "${TEST_DIR}"/image-streams
		;;
	esac
	post_add_model "${controller}" "${model}"
}

# post_add_model does provider specific config required after a new model is added
# and shouldn't be used by any of the tests directly.
post_add_model() {
	local controller model

	controller=${1}
	model=${2}

	ctrl_arg="${controller}:${model}"
	log_file="${controller}-${model}-debug.log"
	if [[ -z ${controller} ]]; then
		ctrl_arg="${model}"
		log_file="${model}.log"
	fi

	# Setup up log tailing on the controller.
	# shellcheck disable=SC2069
	juju debug-log -m "${ctrl_arg}" --replay --tail 2>&1 >"${TEST_DIR}/${log_file}" &
	CMD_PID=$!
	echo "${CMD_PID}" >>"${TEST_DIR}/pids"

	case "${BOOTSTRAP_PROVIDER:-}" in
	"vsphere")
		add_images_for_vsphere
		;;
	esac

	if [[ -n ${MODEL_ARCH} ]]; then
		juju set-model-constraints "arch=${MODEL_ARCH}"
	fi

	if [[ ${BOOTSTRAP_PROVIDER:-} != "k8s" ]]; then
		# As of Juju 4.0, the user's SSH keys are no longer added automatically to newly
		# created models. To ensure that `juju ssh`` works in tests for newly created models,
		# we add the SSH key generated by the Juju client to the model.
		add_client_ssh_key_to_juju_model "${model}"
	fi
}

# add_client_ssh_key_to_juju_model adds juju client public SSH key to the specified Juju model
# usage: add_client_ssh_key_to_juju_model <model_name>
add_client_ssh_key_to_juju_model() {
	local model_name="$1"
	local ssh_key_file="${JUJU_DATA:-${HOME}/.local/share/juju}/ssh/juju_id_ed25519.pub"

	# Check if model name is provided
	if [[ -z $model_name ]]; then
		echo "Error: Invalid usage. The function signature should be: add_client_ssh_key_to_juju_model <model_name>"
		return
	fi

	# Check if the SSH key file exists
	if [[ ! -f $ssh_key_file ]]; then
		echo "Error: SSH key file '$ssh_key_file' not found."
		return
	fi

	# Validate SSH key
	if ssh-keygen -l -f "${ssh_key_file}" >/dev/null 2>&1; then
		output=$(juju add-ssh-key -m "${model_name}" "$(cat "${ssh_key_file}")" 2>&1)
		exit_code=$?
		if [[ $exit_code -ne 0 ]]; then
			echo "Error adding SSH key to model '${model_name}': $output"
			return
		fi
	else
		echo "Error: The file '${ssh_key_file}' does not contain a valid SSH public key."
		return
	fi

	echo "SSH key added successfully to model '${model_name}'."
	return
}

# destroy_model takes a model name and destroys a model. It first checks if the
# model is found before attempting to do so.
#
# ```
# destroy_model <model name>
# ```
destroy_model() {
	if [[ -n ${SKIP_DESTROY} ]]; then
		echo "====> Skipping destroy model"
		return
	fi

	local name

	name=${1}
	shift

	# shellcheck disable=SC2034
	OUT=$(juju models --format=json | jq '.models | .[] | .["short-name"]' | grep "${name}" || true)
	# shellcheck disable=SC2181
	if [[ -z ${OUT} ]]; then
		return
	fi

	output="${TEST_DIR}/${name}-destroy.log"

	echo "====> Destroying juju model ${name}"
	echo "${name}" | xargs -I % timeout "${DESTROY_TIMEOUT}" juju destroy-model --no-prompt --destroy-storage --force % >"${output}" 2>&1 || true
	CHK=$(cat "${output}" | grep -i "ERROR\|Unable to get the model status from the API" || true)
	if [[ -n ${CHK} ]]; then
		printf '\nFound some issues destroying model\n'
		cat "${output}"
		# WARNING. This is a workaround for broken teardown process,
		# where the model is not destroyed properly. This is a known issue
		# and return will be reverted once the Clean-up infrastructure will
		# be fixed. That will help to be move focus on issues in CI tests,
		# rather than on the teardown.
		#		return
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
	if [[ -n ${SKIP_DESTROY} ]]; then
		echo "====> Skipping destroy controller"
		return
	fi

	local name

	name=${1}
	shift

	# shellcheck disable=SC2034
	OUT=$(juju controllers --format=json | jq '.controllers | keys[]' | grep "${name}" || true)
	# shellcheck disable=SC2181
	if [[ -z ${OUT} ]]; then
		OUT=$(juju models --format=json | jq -r '.models | .[] | .["short-name"]' | grep "^${name}$" || true)
		if [[ -z ${OUT} ]]; then
			echo "====> ERROR Destroy controller/model. Unable to locate $(red "${name}")"
			exit 1
		fi
		echo "====> Destroying model ($(green "${name}"))"

		output="${TEST_DIR}/${name}-destroy-model.log"
		echo "${name}" | xargs -I % timeout "${DESTROY_TIMEOUT}" juju destroy-model --no-prompt % >"${output}" 2>&1 || true

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

	output="${TEST_DIR}/${name}-destroy-controller.log"

	echo "====> Destroying juju ($(green "${name}"))"
	if [[ ${KILL_CONTROLLER:-} != "true" ]]; then
		echo "${name}" | xargs -I % timeout "${DESTROY_TIMEOUT}" juju destroy-controller --destroy-all-models --destroy-storage --no-prompt % 2>&1 | OUTPUT "${output}" || true
	else
		echo "${name}" | xargs -I % timeout "${DESTROY_TIMEOUT}" juju kill-controller -t 0 --no-prompt % 2>&1 | OUTPUT "${output}" || true
	fi

	set +e
	CHK=$(cat "${output}" | grep -i "ERROR" || true)
	if [[ -n ${CHK} ]]; then
		printf '\nFound some issues destroying controller\n'
		cat "${output}"
		# WARNING. This is a workaround for broken teardown process,
		# where the model is not destroyed properly. This is a known issue
		# and exit code will be reverted once the Clean-up infrastructure will
		# be fixed. That will help to be move focus on issues in CI tests,
		# rather than on the teardown.
		#		exit 1
	fi
	set_verbosity

	sed -i "/^${name}$/d" "${TEST_DIR}/jujus"
	echo "====> Destroyed juju ($(green "${name}"))"
}

# cleanup_jujus is used to destroy all the known controllers the test suite
# knows about. This is for internal use only and shouldn't be used by any of the
# tests directly.
cleanup_jujus() {
	if [[ -f "${TEST_DIR}/jujus" ]]; then
		echo "====> Cleaning up jujus"

		while read -r juju_name; do
			destroy_controller "${juju_name}"
		done <"${TEST_DIR}/jujus"
		rm -f "${TEST_DIR}/jujus" || true
	fi
	echo "====> Completed cleaning up jujus"
}

introspect_controller() {
	local name

	name=${1}

	if [[ ${BOOTSTRAP_PROVIDER} == "k8s" || ${BOOTSTRAP_PROVIDER} == "microk8s" ]]; then
		echo "====> TODO: Implement introspection for k8s"
		return
	fi

	idents=$(juju machines -m "${name}:controller" --format=json | jq ".machines | keys | .[]")
	if [[ -z ${idents} ]]; then
		return
	fi

	echo "${idents}" | xargs -I % juju ssh -m "${name}:controller" % bash -lc "juju_engine_report" >"${TEST_DIR}/${name}-juju_engine_reports.log" 2>/dev/null
	echo "${idents}" | xargs -I % juju ssh -m "${name}:controller" % bash -lc "juju_goroutines" >"${TEST_DIR}/${name}-juju_goroutines.log" 2>/dev/null
}

remove_controller_offers() {
	local name

	name=${1}

	OUT=$(juju models -c "${name}" --format=json | jq -r '.["models"] | .[] | select(.["is-controller"] == false) | .name' || true)
	if [[ -n ${OUT} ]]; then
		echo "${OUT}" | while read -r model; do
			OUT=$(juju offers -m "${name}:${model}" --format=json | jq -r '.[] | .["offer-url"]' || true)
			echo "${OUT}" | while read -r offer; do
				if [[ -n ${offer} ]]; then
					juju remove-offer --force -y -c "${name}" "${offer}"
					echo "${offer}" >>"${TEST_DIR}/${name}-juju_removed_offers.log"
				fi
			done
		done
	fi
}
