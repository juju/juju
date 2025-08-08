#!/usr/bin/env -S bash -e
export SKIP_DESTROY="${SKIP_DESTROY:-}"
export BOOTSTRAP_BASE="${BOOTSTRAP_BASE:-}"
export BOOTSTRAP_ARCH="${BOOTSTRAP_ARCH:-}"
export BOOTSTRAP_REUSE_LOCAL="${BOOTSTRAP_REUSE_LOCAL:-}"
export BOOTSTRAP_PROVIDER="${BOOTSTRAP_PROVIDER:-lxd}"
export BUILD_ARCH="${BUILD_ARCH:-}"
export MODEL_ARCH="${MODEL_ARCH:-}"
export BUILD_AGENT="${BUILD_AGENT:-false}"
export CURRENT_LTS="ubuntu@22.04"
export DESTROY_TIMEOUT="${DESTROY_TIMEOUT:-15m}"
export RUN_SUBTEST="${RUN_SUBTEST:-}"

OPTIND=1
VERBOSE=1
RUN_ALL="false"
SKIP_LIST=""
RUN_LIST=""
ARTIFACT_FILE=""
OUTPUT_FILE=""

current_pwd=$(pwd)
export CURRENT_DIR="${current_pwd}"

TEST_DIR=$(mktemp -d tmp.XXX | xargs -I % echo "$(pwd)/%")

import_subdir_files() {
	test "$1"
	local file
	for file in "$1"/*.sh; do
		# shellcheck disable=SC1090
		. "$file"
	done
}

import_subdir_files includes

run_test() {
	TEST_SUITE=${1}
    TEST_FUNCTION=${2}

	import_subdir_files "suites/${TEST_SUITE}"

	${TEST_FUNCTION}
}

run_test "${1}" "${2}"
