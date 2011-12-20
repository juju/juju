#!/bin/sh
set -e
root=`pwd`

goto() {
	cd "$@"
	echo
	echo ----- entering $*
}

dirs="log schema charm environs environs/ec2 environs/jujutest store"

for dir in $dirs; do
	goto "$root/$dir"
	make clean
done

for dir in $dirs; do
	goto "$root/$dir"
	make install
done

for dir in $dirs; do
	goto "$root/$dir"
	gotest
done

echo DONE
