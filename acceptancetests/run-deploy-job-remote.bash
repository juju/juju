#!/bin/bash
# Script to run run-deploy-job compatibly with workspace-runner.  It takes
# revision_build and job_name as first parameters, the rest like
# run-deploy-job.  Workspace is assumed to be pwd.
set -eux
export revision_build=$1
export JOB_NAME=$2
export WORKSPACE=$(pwd)
SCRIPTS=$(readlink -f $(dirname $0))
shift 2
$SCRIPTS/run-deploy-job.bash "$@"
