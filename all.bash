#!/bin/sh
set -e
root=`pwd`

goto() {
	cd "$@"
	echo building $*
}

dirs="environ environ/jujutest environ/ec2 schema charm"

for i in $dirs; do
	goto "$root/$i"
	make clean
	gotest
	make install
done
echo DONE
