#!/bin/bash
#--------------------------------------------
# This file is managed by Juju
#--------------------------------------------
#                                       
# Copyright 2009,2012 Canonical Ltd.
# Author: Tom Haddon

# These should be config options at some stage
CURRQthrsh=0
MAXQthrsh=100

AUTH=$(grep -r "stats auth" /etc/haproxy | head -1 | awk '{print $4}')

HAPROXYSTATS=$(/usr/lib/nagios/plugins/check_http -a ${AUTH} -I 127.0.0.1 -p 8888 -u '/;csv' -v)

for BACKEND in $(echo $HAPROXYSTATS| xargs -n1 | grep BACKEND | awk -F , '{print $1}')
do
    CURRQ=$(echo "$HAPROXYSTATS" | grep $BACKEND | grep BACKEND | cut -d , -f 3)
    MAXQ=$(echo "$HAPROXYSTATS"  | grep $BACKEND | grep BACKEND | cut -d , -f 4)

    if [[ $CURRQ -gt $CURRQthrsh || $MAXQ -gt $MAXQthrsh ]] ; then
        echo "CRITICAL: queue depth for $BACKEND - CURRENT:$CURRQ MAX:$MAXQ"
        exit 2
    fi
done

echo "OK: All haproxy queue depths looking good"
exit 0

