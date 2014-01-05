#!/bin/bash
exitstatus=0
for i in $(go list -f '{{.Dir}}' launchpad.net/juju-core/...)
do
	src=$i/*_test.go
	# The -s flag is needed to suppress errors when
	# the above pattern does not match any files.
	if grep -s -q -l 'launchpad.net/gocheck' $src &&
		! egrep -l -q 'gc\.TestingT|testing\.(\w*)MgoTestPackage' $src
	then
		# There are _test.go files that use gocheck but
		# don't call gocheck.TestingT.
		echo $i uses gocheck but never calls TestingT
		exitstatus=1
	fi
done
exit $exitstatus
