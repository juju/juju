#!/bin/bash

CONFIG=$1
if [[ -z $CONFIG ]]; then
    echo $0 path/to/s3config
    exit 1
fi

# Never delete the juju-ci4 control bucket.
CI_CONTROL_BUCKET=$(juju get-env -e juju-ci4 control-bucket)
# This cloud be 2 days ago because hours are not involved.
YESTERDAY=$(($(date +"%y%m%d") - 2))

# Get the list of buckets that are 32 hex chars long except the control-bucket.
BUCKETS=$(s3cmd -c $CONFIG ls |
    grep -v $CI_CONTROL_BUCKET |
    grep -E 's3://[0-9a-f]{32,32}' |
    cut -d ' ' -f 1,4 |
    sed -r 's, ,_,g')

for bucket in $BUCKETS; do
    name=$(echo "$bucket" | cut -d '_' -f 2)
    datestamp=$(echo "$bucket" | cut -d '_' -f 1)
    then=$(date -d "$datestamp" +"%y%m%d")
    if [[ $((then)) -le $((YESTERDAY)) ]]; then
        echo "Deleting $name" created "$datestamp"
        s3cmd -c $CONFIG del --recursive --force $name
        s3cmd -c $CONFIG rb $name
    else
        echo "Skipping $name" created "$datestamp"
    fi
done
