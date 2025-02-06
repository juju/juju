#!/usr/bin/env bash
set -e

go test -c -race
PKG=$(basename $(pwd))

# Counter
C=0
START_TIME=$SECONDS

while true ; do
        C=$((C+1))
        export GOTRACEBACK=all
        export GOMAXPROCS=$[ 1 + $[ RANDOM % 128 ]]
        ./$PKG.test $@ 2>&1
        ELAPSED_TIME=$(($SECONDS - $START_TIME))
        echo "$C successful runs in $(($ELAPSED_TIME/60)) min $(($ELAPSED_TIME%60)) sec"
done
