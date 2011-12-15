#!/bin/sh
set -e
root=`pwd`

goto() {
	cd "$@"
	echo building $*
}

dirs="environs environs/jujutest environs/ec2 schema charm"

for i in $dirs; do
	goto "$root/$i"
	make clean
	gotest
	make install
done
echo DONE
