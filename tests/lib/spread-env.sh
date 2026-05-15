#!/usr/bin/env bash
# spread-env.sh — Adapter for running Juju test libraries under spread.
#
# Source this at the top of any spread task script to set up the
# environment variables that the shared libraries (juju.sh, wait-for.sh,
# etc.) expect.

# Ensure the juju binary built by the project-level prepare is on PATH.
export PATH="${SPREAD_PATH}/../go/bin:${PATH}"

# TEST_DIR is used by libraries for temp files and logs.
# In spread, each task gets its own working directory; we use a temp dir
# under /tmp scoped to the current task.
if [ -z "${TEST_DIR:-}" ]; then
    TEST_DIR=$(mktemp -d "/tmp/spread-juju-XXXXXX")
    export TEST_DIR
fi

# Ensure the test dir cleanup file exists (used by cleanup.sh).
touch "${TEST_DIR}/cleanup"

# VERBOSE controls set_verbosity behaviour. Default to basic mode.
export VERBOSE="${VERBOSE:-1}"

# TEST_CURRENT is used for log file naming.
export TEST_CURRENT="${SPREAD_TASK##*/}"

# CURRENT_DIR is expected by some libraries to be the project root.
export CURRENT_DIR="${SPREAD_PATH}"

# Source core libraries that most tasks need.
. "$TESTSLIB/verbose.sh"
. "$TESTSLIB/colors.sh"
. "$TESTSLIB/output.sh"
. "$TESTSLIB/check.sh"
. "$TESTSLIB/cleanup.sh"
. "$TESTSLIB/pids.sh"
. "$TESTSLIB/juju.sh"
. "$TESTSLIB/wait-for.sh"
. "$TESTSLIB/random.sh"
. "$TESTSLIB/retry.sh"
