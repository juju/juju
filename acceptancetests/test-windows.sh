#!/bin/bash
#This script allows you to run arbitrary commands on windows slaves
#Usage: comannd_to_run host (tarball or revision id)
set -eux

runcmd() {
    #Make payload to run
    payload="$ts.bat"
    echo "cd C:\\Users\\Administrator\\payloads\\$ts" > $payload
    echo "$1" >> $payload
    #Ensure the error is returned if the command fails
    echo "if errorlevel 1 (" >> $payload
    echo "   exit /b %errorlevel%" >> $payload
    echo ")" >> $payload
    scp -i $JUJU_HOME/staging-juju-rsa  -oStrictHostKeyChecking=no $payload $host:C:\\Users\\Administrator\\payloads\\
    #Run test command via ssh
    ssh $host C:\\Users\\Administrator\\payloads\\$payload
}

cmd=$1
host=$2
ts=$(date +%s)
echo "Running $cmd on $host as payload $ts"
runcmd "mkdir C:\\Users\\Administrator\\payloads\\$ts"

if [ $# -eq 3 ]; then
    if [[ $3 == *"tar"* ]]; then
        tarfile=$3
        echo "Using $tarfile as tarball"
    else
        echo "Retrieving tarball for revision build $3"
        tarfile=$($SCRIPTS/s3ci.py get $3 build-revision "juju-core_.*.tar.gz" .)
        tarfile=$(basename $tarfile)
    fi
    #Copy tarball over via scp
    scp -i $JUJU_HOME/staging-juju-rsa  -oStrictHostKeyChecking=no $tarfile $host:C:\\Users\\Administrator\\payloads\\$ts\\$tarfile
else
    echo "Not using juju tarball for test"
fi

runcmd $cmd
