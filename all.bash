#!/bin/sh
set -e
root=`pwd`

goto() {
	cd "$@"
	echo building $*
}

dirs="charm juju juju/jujutest juju/ec2 schema"

for i in $dirs; do
	goto "$root/$i"
	make clean
	gotest
	make install
done
echo DONE
