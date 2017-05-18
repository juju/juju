#!/bin/bash
set -eu

if [[ -z ${JENKINS_USER:-} || -z ${JENKINS_PASSWORD:-} ]]; then
    echo "JENKINS_USER and JENKINS_PASSWORD must be defined."
    exit 1
fi
ACTION=$1
if [[ ! $ACTION =~ ^(enable|disable)$ ]]; then
    echo "Expected $0 <enable|disable>."
    exit 1
fi
HOURS=${2:-1}


CREDS="$JENKINS_USER:$JENKINS_PASSWORD"
JOB="http://$CREDS@juju-ci.vapour.ws:8080/job/build-revision"


# Empty the "C" queue. There can be only 1 or 0 jobs in it.
for job in $(atq -q c | cut  -f 1); do
    atrm $job
done

if [[ $ACTION == 'enable' ]]; then
    curl --data enable "$JOB/enable"
else
    curl --data disable "$JOB/disable"
    echo "Build-revision will be enabled in $HOURS hour"
    echo curl --data enable "$JOB/enable" | at -q c now +$HOURS hours
fi
