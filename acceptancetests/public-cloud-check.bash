#!/bin/bash
set -e

# A simple test that ensures that the streams stored public-clouds.yaml match
# what is built into the Juju under test.
# Basically setup a pristine JUJU_DATA, run juju upgrade-clouds, if a public-clouds.yaml exists fail.

function usage() {
    echo "Usage: $0 <path to juju binary>"
}

function cleanup() {
    if [ ! -z "${test_tmpdir+x}" ]; then
        rm -fr ${test_tmpdir}
    fi
}

trap cleanup EXIT

if [ "$#" -ne 1 ]; then
    usage
    exit 1
fi

juju_bin=$1
test_tmpdir=$(mktemp -d)
export JUJU_DATA="${test_tmpdir}"

${juju_bin} --show-log update-public-clouds --client

# The public-clouds.yaml published must match what is built into the Juju binary
# under test.
public_clouds_file="${JUJU_DATA}/public-clouds.yaml"
if [ -f ${public_clouds_file} ]; then
    echo -e "\nFAIL: File exists when it shouldn't: ${public_clouds_file}"
    exit 1
fi

echo -e "\nPASS"
exit 0
