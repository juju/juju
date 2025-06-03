#!/usr/bin/env bash

CONTROLLERS="10.132.183.12,10.132.183.101,10.132.183.127"
UUID="a5fa603d-5385-4ffc-85b5-79f6ba0eafab"
UNIT="ubuntu/0"
PASSWORD="v2Vf0BzpFOvwIZyZa9XxLqgL"

for i in `seq -f%02g 0 99`; do
    x="";
    for j in `seq -f%02g 0 39`; do
        for u in `seq 0 2 | shuf`; do
            x="$x a$i$j/$u"
        done
    done
    echo "$x"


    ./leadershipclaimer --quiet --hosts=$CONTROLLERS --uuid=$UUID \
	    --claimtime=1m --renewtime=30s \
	    --unit $UNIT --password $PASSWORD \
        $x >> claims.log &
done
