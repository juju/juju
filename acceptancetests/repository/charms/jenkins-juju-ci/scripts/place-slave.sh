#!/bin/bash

SLAVE_JAR=/var/lib/jenkins/bin/slave.jar
RUN_JAR=/var/run/jenkins/slave.jar
JENKINS_URL=$1

if [[ ! -f /var/lib/jenkins/bin/slave.jar ]]; then
    if [[ -z "$JENKINS_URL" ]]; then
        echo URL of jenkins server must be provided
        exit 1
    fi
    echo "Downloading slave.jar from $JENKINS_URL"
    wget -q -O $SLAVE_JAR $JENKINS_URL/jnlpJars/slave.jar

    if [[ ! -f $SLAVE_JAR ]] ; then
        echo "Failed to download slave.jar, no file"
        exit 1
    fi
    if [[ $(stat -c%s $SLAVE_JAR) == "0" ]]; then
        echo "Failed to download slave.jar, empty file"
        exit 1
    fi
    sudo chown jenkins:jenkins $SLAVE_JAR
fi

if [[ ! -d  /var/run/jenkins ]]; then
    mkdir /var/run/jenkins
fi
cp $SLAVE_JAR $RUN_JAR
sudo chown jenkins:jenkins $RUN_JAR
