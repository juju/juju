#!/bin/bash

CONFIG=$1
if [[ -z $CONFIG ]]; then
    echo $0 path/to/s3config
    exit 1
fi

# Never delete the juju-ci4 control bucket.
CI_CONTROL_BUCKET=${2:-$(juju get-env -e juju-ci4 control-bucket)}
# This could be almost 3 hours ago.
HOURS_AGO=$(($(date +"%Y%m%d%H") - 2))

# Get the list of buckets that are 32 hex chars long except the control-bucket.
BUCKETS=$(s3cmd -c $CONFIG ls |
    grep -v $CI_CONTROL_BUCKET |
    grep -E 's3://[0-9a-f]{32,32}' |
    cut -d ' ' -f 1,2,4 |
    sed -r 's,:.* ,_,g; s, ,:,g;')

for bucket in $BUCKETS; do
    name=$(echo "$bucket" | cut -d '_' -f 2)
    datestamp=$(echo "$bucket" | cut -d '_' -f 1 | sed -r 's,:, ,')
    then=$(date -d "$datestamp" +"%Y%m%d%H")
    if [[ $((then)) -le $((HOURS_AGO)) ]]; then
        echo "Deleting $name" created "$datestamp"
        s3cmd -c $CONFIG del --recursive --force $name
        s3cmd -c $CONFIG rb $name
    else
        echo "Skipping $name" created "$datestamp"
    fi
done
