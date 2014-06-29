#!/bin/bash

function sdc {
    local alg=rsa-sha256
    local keyId=/$SDC_ACCOUNT/keys/$SDC_KEY_ID
    local now=$(date -u "+%a, %d %h %Y %H:%M:%S GMT")
    local sig=$(echo -n "date: $now" |
        openssl dgst -sha256 -sign $JUJU_HOME/id_rsa |
        openssl enc -e -a |
        tr -d '\n')
    curl -sS $SDC_URL"$@" -H "Date: $now" \
        -H "Authorization: Signature keyId=\"$keyId\",algorithm=\"$alg\",signature=\"$sig\""
}

sdc $@
