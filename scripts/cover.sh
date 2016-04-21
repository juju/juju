#!/bin/bash
# Run test coverage recursively from the current directory and merge the
# coverage profile.
 
# Set mode to count in case we want to analyze that...
echo "mode: count" > profile.cov

# Ignore .git/ and directories with leading underscores.
for dir in $(find . -maxdepth 10 -not -path './.git*' -not -path '*/_*' -type d);
do
if ls $dir/*.go &> /dev/null; then
    go test -covermode=count -coverprofile=$dir/profile.tmp $dir
    if [ -f $dir/profile.tmp ]
    then
        cat $dir/profile.tmp | tail -n +2 >> profile.cov
        rm $dir/profile.tmp
    fi
fi
done

go tool cover -func profile.cov

