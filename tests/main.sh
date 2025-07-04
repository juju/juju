#!/bin/bash -e
[ -n "${GOPATH:-}" ] && export "PATH=${PATH}:${GOPATH}/bin"

# Always ignore SC2230 ('which' is non-standard. Use builtin 'command -v' instead.)
export SHELLCHECK_OPTS="-e SC2230 -e SC2039 -e SC2028 -e SC2002 -e SC2005 -e SC2001 -e SC2263"
export BOOTSTRAP_REUSE_LOCAL="${BOOTSTRAP_REUSE_LOCAL:-}"
export BOOTSTRAP_REUSE="${BOOTSTRAP_REUSE:-false}"
export BOOTSTRAP_PROVIDER="${BOOTSTRAP_PROVIDER:-lxd}"
export BOOTSTRAP_CLOUD="${BOOTSTRAP_CLOUD:-}"
export BOOTSTRAP_SERIES="${BOOTSTRAP_SERIES:-}"
export BOOTSTRAP_ARCH="${BOOTSTRAP_ARCH:-}"
export BUILD_ARCH="${BUILD_ARCH:-}"
export MODEL_ARCH="${MODEL_ARCH:-}"
export BUILD_AGENT="${BUILD_AGENT:-false}"
export RUN_SUBTEST="${RUN_SUBTEST:-}"
export CURRENT_LTS="jammy"

current_pwd=$(pwd)
export CURRENT_DIR="${current_pwd}"

OPTIND=1
VERBOSE=1
RUN_ALL="false"
SKIP_LIST=""
RUN_LIST=""
ARTIFACT_FILE=""
OUTPUT_FILE=""

import_subdir_files() {
	test "$1"
	local file
	for file in "$1"/*.sh; do
		# shellcheck disable=SC1090
		. "$file"
	done
}

import_subdir_files includes

# If adding a test suite, then ensure to add it here to be picked up!
# Please keep these in alphabetic order.
TEST_NAMES="agents \
            appdata \
            authorized_keys \
            backup \
            bootstrap \
            branches \
            caasadmission \
            charmhub \
            cli \
            cloud_azure \
            constraints \
            controller \
            coslite \
            credential \
            ck \
            deploy \
            deploy_aks \
            deploy_caas \
            firewall \
            hooks \
            hooktools \
            kubeflow \
            machine \
            manual \
            model \
            network \
            ovs_maas \
            refresh \
            relations \
            resources \
            secrets_iaas \
            secrets_k8s \
            sidecar \
            smoke \
            spaces_ec2 \
            static_analysis \
            storage \
            storage_k8s \
            unit \
            upgrade \
            user"

# Show test suites, can be used to test if a test suite is available or not.
show_test_suites() {
	output=""
	for test in ${TEST_NAMES}; do
		name=$(echo "${test}" | sed -E "s/^run_//g" | sed -E "s/_/ /g")
		# shellcheck disable=SC2086
		output="${output}\n${test}"
	done
	echo -e "${output}" | column -t -s "|"
	exit 0
}

show_help() {
	version=$(juju version)
	echo ""
	echo "$(red 'Juju test suite')"
	echo "¯¯¯¯¯¯¯¯¯¯¯¯¯¯¯"
	# shellcheck disable=SC2016
	echo 'Juju tests suite expects you to have a Juju available on your $PATH,'
	echo "so that if a tests needs to bootstrap it can just use that one"
	echo "directly."
	echo ""
	echo "Juju Version:"
	echo "¯¯¯¯¯¯¯¯¯¯¯¯¯"
	echo "Using juju version: $(green "${version}")"
	echo ""
	echo "Usage:"
	echo "¯¯¯¯¯¯"
	echo "Flags should appear $(red 'before') arguments."
	echo ""
	echo "cmd [-h] [-v] [-A] [-s test] [-a file] [-x file] [-r] [-l controller] [-p provider type <lxd|aws|google|azure|manual|microk8s|vsphere|maas>]"
	echo ""
	echo "    $(green './main.sh -h')        Display this help message"
	echo "    $(green './main.sh -v')        Verbose and debug messages"
	echo "    $(green './main.sh -V')        Verbose and debug messages with all commands printed"
	echo "    $(green './main.sh -A')        Run all the test suites"
	echo "    $(green './main.sh -s')        Skip tests using a comma seperated list"
	echo "    $(green './main.sh -a')        Create an artifact file"
	echo "    $(green './main.sh -x')        Output file from streaming the output"
	echo "    $(green './main.sh -r')        Reuse bootstrapped controller between testing suites"
	echo "    $(green './main.sh -l')        Local bootstrapped controller name to reuse"
	echo "    $(green './main.sh -p')        Bootstrap provider to use when bootstrapping <lxd|aws|google|azure|manual|k8s|openstack|vsphere|maas>"
	echo "                                     vsphere assumes juju boston vsphere for image metadata generation"
	echo "                                     openstack assumes providing image data directly is not required"
	echo "    $(green './main.sh -c')        Cloud name to use when bootstrapping, must be one of provider types listed above"
	echo "    $(green './main.sh -R')        Region to use with cloud"
	echo "    $(green './main.sh -S')        Bootstrap series to use <default is host>, priority over -l"
	echo ""
	echo "Tests:"
	echo "¯¯¯¯¯¯"
	echo "Available tests:"
	echo ""

	# Let's use the TEST_NAMES to print out what's available
	output=""
	for test in ${TEST_NAMES}; do
		name=$(echo "${test}" | sed -E "s/^run_//g" | sed -E "s/_/ /g")
		# shellcheck disable=SC2086
		output="${output}\n    $(green ${test})|Runs the ${name} tests"
	done
	echo -e "${output}" | column -t -s "|"

	echo ""
	echo "Examples:"
	echo "¯¯¯¯¯¯¯¯¯"
	echo "Run a singular test:"
	echo ""
	echo "    $(green './main.sh static_analysis test_static_analysis_go')"
	echo ""
	echo "Run static analysis tests, but skip the go static analysis tests:"
	echo ""
	echo "    $(green './main.sh -s test_static_analysis_go static_analysis')"
	echo ""
	echo "Run a more verbose output and save that to an artifact tar (it"
	echo "requires piping the output from stdout and stderr into a output.log,"
	echo "which is then copied into the artifact tar file on test cleanup):"
	echo ""
	echo "    $(green './main.sh -v -a artifact.tar.gz -x output.log 2>&1|tee output.log')"
	exit 1
}

while getopts "hH?vAs:a:x:rl:p:c:R:S:V" opt; do
	case "${opt}" in
	h | \?)
		show_help
		;;
	H)
		show_test_suites
		;;
	v)
		VERBOSE=2
		# shellcheck disable=SC2262
		alias juju="juju --debug"
		;;
	V)
		VERBOSE=11
		# shellcheck disable=SC2262
		alias juju="juju --debug"
		;;
	A)
		RUN_ALL="true"
		;;
	s)
		SKIP_LIST="${OPTARG}"
		;;
	a)
		ARTIFACT_FILE="${OPTARG}"
		;;
	x)
		OUTPUT_FILE="${OPTARG}"
		;;
	r)
		export BOOTSTRAP_REUSE="true"
		;;
	l)
		export BOOTSTRAP_REUSE_LOCAL="${OPTARG}"
		export BOOTSTRAP_REUSE="true"

		CLOUD=$(juju show-controller "${OPTARG}" --format=json 2>/dev/null | jq -r ".[\"${OPTARG}\"] | .details | .cloud")
		PROVIDER=$(juju clouds --client --all --format=json 2>/dev/null | jq -r ".[\"${CLOUD}\"] | .type")
		export BOOTSTRAP_PROVIDER="${PROVIDER}"
		export BOOTSTRAP_CLOUD="${CLOUD}"
		;;
	p)
		export BOOTSTRAP_PROVIDER="${OPTARG}"
		;;
	c)
		PROVIDER=$(juju clouds --client --all --format=json 2>/dev/null | jq -r ".[\"${OPTARG}\"] | .type")
		export BOOTSTRAP_PROVIDER="${PROVIDER}"
		CLOUD="${OPTARG}"
		export BOOTSTRAP_CLOUD="${CLOUD}"
		;;
	R)
		export BOOTSTRAP_REGION="${OPTARG}"
		;;
	S)
		export BOOTSTRAP_SERIES="${OPTARG}"
		;;
	*)
		echo "Unexpected argument ${opt}" >&2
		exit 1
		;;
	esac
done

shift $((OPTIND - 1))
[[ ${1:-} == "--" ]] && shift

export VERBOSE="${VERBOSE}"
export SKIP_LIST="${SKIP_LIST}"

if [[ $# -eq 0 ]]; then
	if [[ ${RUN_ALL} != "true" ]]; then
		echo "$(red '---------------------------------------')"
		echo "$(red 'Run with -A to run all the test suites.')"
		echo "$(red '---------------------------------------')"
		echo ""
		show_help
	fi
fi

echo ""

echo "==> Checking for dependencies"
check_dependencies curl jq yq shellcheck expect

if [[ ${USER:-'root'} == "root" ]]; then
	echo "The testsuite must not be run as root." >&2
	exit 1
fi

JUJU_FOUND=0
which juju &>/dev/null || JUJU_FOUND=$?
if [[ $JUJU_FOUND == 0 ]]; then
	echo "==> Using Juju located at $(which juju)"
else
	# shellcheck disable=SC2016
	echo '==> WARNING: no Juju found on $PATH'
fi

cleanup() {
	# Allow for failures and stop tracing everything
	set +ex

	# Allow for inspection
	if [[ -n ${TEST_INSPECT:-} ]]; then
		if [[ ${TEST_RESULT} != "success" ]]; then
			echo "==> TEST DONE: ${TEST_CURRENT_DESCRIPTION}"
		fi
		echo "==> Test result: ${TEST_RESULT}"
		echo "Tests Completed (${TEST_RESULT}): hit enter to continue"

		# shellcheck disable=SC2034
		read -r nothing
	fi

	echo "==> Cleaning up"

	archive_logs "partial"

	cleanup_pids
	cleanup_jujus
	cleanup_funcs

	echo ""
	if [[ ${TEST_RESULT} != "success" ]]; then
		echo "==> TESTS DONE: ${TEST_CURRENT_DESCRIPTION}"
		if [[ -f "${TEST_DIR}/${TEST_CURRENT}.log" ]]; then
			echo "==> RUN OUTPUT: ${TEST_CURRENT}"
			cat "${TEST_DIR}/${TEST_CURRENT}.log" | sed 's/^/    | /g'
			echo ""
		fi
	fi
	echo "==> Test result: ${TEST_RESULT}"

	archive_logs "full"

	if [ "${TEST_RESULT}" = "success" ]; then
		rm -rf "${TEST_DIR}"
		echo "==> Tests Removed: ${TEST_DIR}"
	fi

	echo "==> TEST COMPLETE"
}

# Move any artifacts to the chosen location
archive_logs() {
	if [[ -z ${ARTIFACT_FILE} ]]; then
		return
	fi

	archive_type="${1}"

	echo "==> Test ${archive_type} artifact: ${ARTIFACT_FILE}"
	if [[ -f ${OUTPUT_FILE} ]]; then
		cp "${OUTPUT_FILE}" "${TEST_DIR}"
	fi
	TAR_OUTPUT=$(tar -C "${TEST_DIR}" --transform s/./artifacts/ -zcvf "${ARTIFACT_FILE}" ./ 2>&1)
	# shellcheck disable=SC2181
	if [[ $? -eq 0 ]]; then
		echo "==> Test ${archive_type} artifact: COMPLETED"
	else
		echo "${TAR_OUTPUT}"
		TEST_RESULT=failure
	fi

}

TEST_CURRENT=setup
TEST_RESULT=failure

trap cleanup EXIT HUP INT TERM

# Setup test directory
TEST_DIR=$(mktemp -d tmp.XXX | xargs -I % echo "$(pwd)/%")

run_test() {
	TEST_CURRENT=${1}
	TEST_CURRENT_DESCRIPTION=${2:-${1}}
	TEST_CURRENT_NAME=${TEST_CURRENT#"test_"}

	if [[ -n ${4} ]]; then
		TEST_CURRENT=${4}
	fi

	import_subdir_files "suites/${TEST_CURRENT_NAME}"

	# shellcheck disable=SC2046,SC2086
	echo "==> TEST BEGIN: ${TEST_CURRENT_DESCRIPTION} ($(green $(basename ${TEST_DIR})))"
	START_TIME=$(date +%s)
	${TEST_CURRENT}
	END_TIME=$(date +%s)

	echo "==> TEST DONE: ${TEST_CURRENT_DESCRIPTION} ($((END_TIME - START_TIME))s)"
}

# allow for running a specific set of tests
if [[ $# -gt 0 ]]; then
	# shellcheck disable=SC2143
	if [[ "$(echo "${2}" | grep -E "^run_")" ]]; then
		TEST="$(grep -lr "run \"${2}\"" "suites/${1}" | xargs sed -rn 's/.*(test_\w+)\s+?\(\)\s+?\{/\1/p')"
		if [[ -z ${TEST} ]]; then
			echo "==> Unable to find parent test for ${2}."
			echo "    Try and run the parent test directly."
			exit 1
		fi

		export RUN_SUBTEST="${2}"
		echo "==> Running subtest: ${2}"
		run_test "test_${1}" "" "" "${TEST}"
		TEST_RESULT=success
		exit
	fi
	# shellcheck disable=SC2143
	if [[ "$(echo "${2}" | grep -E "^test_")" ]]; then
		TEST="$(grep -lr "${2}" "suites/${1}")"
		if [[ -z ${TEST} ]]; then
			echo "==> Unable to find test ${2} in ${1}."
			echo "    Try and run the test suite directly."
			exit 1
		fi

		export RUN_LIST="test_${1},${2}"
		echo "==> Running subtest ${2} for ${1} suite"
		run_test "test_${1}" "${1}" "" ""
		TEST_RESULT=success
		exit
	fi

	run_test "test_${1}" "" "$@" ""
	TEST_RESULT=success
	exit
fi

for test in ${TEST_NAMES}; do
	name=$(echo "${test}" | sed -E "s/^run_//g" | sed -E "s/_/ /g")
	run_test "test_${test}" "${name}" "" ""
done

TEST_RESULT=success
