#!/bin/bash
#This script allows you to run arbitrary commands on windows slaves
#You can also specify a tarball to be used as part of the payload
#Usage: host (tarball or revision id or cmd_to_run)
set -eux
IFS='%'

runcmd() {
    #Make payload to run
    payload="$ts.bat"
    echo "cd C:\\Users\\Administrator\\payloads\\$ts" > $payload
    echo "$1" >> $payload
    #Ensure the error is returned if the command fails
    echo "if errorlevel 1 (" >> $payload
    echo "   exit /b %errorlevel%" >> $payload
    echo ")" >> $payload
    scp -i $JUJU_HOME/staging-juju-rsa -oStrictHostKeyChecking=no $payload $host:C:\\Users\\Administrator\\payloads\\
    #Run test command via ssh
    ssh $host C:\\Users\\Administrator\\payloads\\$payload
}

host=$1
ts=$(date +%s)
tarfile=''
echo "Running on $host as payload $ts"
runcmd "mkdir C:\\Users\\Administrator\\payloads\\$ts"

if [[ $2 == *"tar"* ]]; then
    tarfile=$2
    echo "Using $tarfile as tarball"
elif [ "$2" -eq "$2" ] 2>/dev/null; then
    echo "Retrieving tarball for revision build $2"
    tarfile=$($SCRIPTS/s3ci.py get $2 build-revision "juju-core_.*.tar.gz" .)
    tarfile=$(basename $tarfile)
else
    cmd=$2
fi

if [ -n "$tarfile" ]; then
    #Copy tarball over via scp
    scp -i $JUJU_HOME/staging-juju-rsa -oStrictHostKeyChecking=no $tarfile $host:C:\\Users\\Administrator\\payloads\\$ts\\$tarfile
    cmd="python C:\Users\Administrator\juju-ci-tools\gotesttarfile.py -v -g go.exe -r C:\\Users\\Administrator\\payloads\\$ts\\$tarfile -p github.com/juju/juju/cmd"
fi

runcmd $cmd
