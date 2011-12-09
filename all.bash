#!/bin/sh
set -e
root=`pwd`

goto() {
	cd "$@"
	echo building $*
}

dirs="juju juju/ec2 schema  juju/jujutest charm "

for i in $dirs; do
	goto "$root/$i"
	make clean
	gotest
	make install
done
echo DONE
