#!/bin/bash
exitstatus=0
for i in $(go list -f '{{.Dir}}' launchpad.net/juju-core/...)
do
	case $i in
	*testing)
		;;
	*)
		src=$i/*.go
		if grep -q -l 'launchpad.net/gocheck' $src &&
			! egrep -l -q 'gc\.TestingT|testing\.MgoTestPackage' $src
		then
			echo $i uses gocheck but never calls TestingT
			exitstatus=1
		fi
		;;
	esac
done
exit $exitstatus
