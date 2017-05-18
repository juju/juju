#!/bin/bash

function manta {
    local alg=rsa-sha256
    local keyId=/$MANTA_USER/keys/$MANTA_KEY_ID
    local now=$(date -u "+%a, %d %h %Y %H:%M:%S GMT")
    #local now="Sat, 19 Apr 2014 04:26:17 GMT"
    local sig=$(echo -n "date: $now" |
        openssl dgst -sha256 -sign $JUJU_HOME/id_rsa |
        openssl enc -e -a |
        tr -d '\n')
    curl -sS $MANTA_URL"$@" -H "Date: $now" \
        -H "Authorization: Signature keyId=\"$keyId\",algorithm=\"$alg\",signature=\"$sig\""
}

manta $@
