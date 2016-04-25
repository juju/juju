#!/bin/bash
# haproxy-to-stasd: query haproxy CSV status page, transform it
# to stdout suitable for statsd
#
# Author: JuanJo Ciarlante <jjo@canonical.com>
# Copyright 2012, Canonical Ltd.
# License: GPLv3
set -u
PREFIX=${1:?missing statsd node prefix, e.g.: production.host.${HOSTNAME}.haproxy.stats}
PERIOD=${2:?missing period, e.g.: 10min}
HOSTPORT=${3:?missing haproxy hostport, e.g.: localhost:10000}
HTTPAUTH=${4:?missing httpauth, e.g.: user:pass}

TSTAMP="$(date +%s)"

# Filter only numeric metrics, cleanup to be only <var> <value>
get_metrics() {
    curl -s "http://${HTTPAUTH}@${HOSTPORT}/;csv;norefresh"| \
        awk -v FS=, '/^#/{ for(i=1;i<NF;i++) fieldname[i]=$i;}
/^[^#]/{ for(i=3;i<NF;i++) if(length($i) > 0) printf("%s.%s.%s %d\n", $1, $2, fieldname[i], $i);}'
}

# Add statsd-isms to <var> <value> lines
statsdify() {
    sed -r "s/([^ ]+) (.*)/${PREFIX}.\1.${PERIOD}:\2|g/"
}
get_metrics | statsdify
