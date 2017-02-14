#!/bin/bash
#--------------------------------------------
# This file is managed by Juju
#--------------------------------------------
#
# Copyright 2009,2012 Canonical Ltd.
# Author: Tom Haddon

CRITICAL=0
NOTACTIVE=''
LOGFILE=/var/log/nagios/check_haproxy.log
AUTH=$(grep -r "stats auth" /etc/haproxy | awk 'NR=1{print $4}')

typeset -i N_INSTANCES=0
for appserver in $(awk '/^\s+server/{print $2}' /etc/haproxy/haproxy.cfg)
do
    N_INSTANCES=N_INSTANCES+1
    output=$(/usr/lib/nagios/plugins/check_http -a ${AUTH} -I 127.0.0.1 -p 8888 -u '/;csv' --regex=",${appserver},.*,UP.*" -e ' 200 OK')
    if [ $? != 0 ]; then
        date >> $LOGFILE
        echo $output >> $LOGFILE
        /usr/lib/nagios/plugins/check_http -a ${AUTH} -I 127.0.0.1 -p 8888 -u '/;csv' -v | grep ",${appserver}," >> $LOGFILE 2>&1
        CRITICAL=1
        NOTACTIVE="${NOTACTIVE} $appserver"
    fi
done

if [ $CRITICAL = 1 ]; then
    echo "CRITICAL:${NOTACTIVE}"
    exit 2
fi

echo "OK: All haproxy instances ($N_INSTANCES) looking good"
exit 0
