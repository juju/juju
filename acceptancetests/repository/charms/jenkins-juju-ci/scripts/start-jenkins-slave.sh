#!/bin/sh
test -n "$JENKINS_URL" || . /etc/default/jenkins-slave
test -n "$JENKINS_URL" || { stop; exit 0; }
mkdir $JENKINS_RUN  > /dev/null 2>&1  || true
chown -R $JENKINS_USER $JENKINS_RUN || true
$JENKINS_HOME/bin/place-slave.sh $JENKINS_URL
/sbin/start-stop-daemon --start -c $JENKINS_USER \
    --exec $JAVA --name jenkins-slave \
    -- $JAVA_ARGS -jar $JENKINS_RUN/slave.jar $JENKINS_ARGS
