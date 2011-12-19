#!/bin/sh
set -e
root=`pwd`

goto() {
	cd "$@"
	echo
	echo ----- building $*
}

dirs="schema environs environs/ec2 environs/jujutest charm"

for dir in $dirs; do
	goto "$root/$dir"
	make clean
	make install
done

for dir in $dirs; do
	goto "$root/$dir"
	gotest
done

echo DONE
