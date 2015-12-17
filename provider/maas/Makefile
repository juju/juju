all: bridgescript.go

# TODO add bridgescript_doc.go that explains (in a package comment)
# what this script (add-juju-bridge.py) does, and why, and how to
# change it.

bridgescript.go: add-juju-bridge.py Makefile
	$(RM) $@
	echo -n '// This file is auto generated. Edits will be lost.\n\n' >> $@
	echo -n 'package maas\n\n' >> $@
	echo -n '//go:generate make -q\n\n' >> $@
	echo -n 'import "path"\n\n' >> $@
	echo -n 'const bridgeScriptName = "add-juju-bridge.py"\n\n' >> $@
	echo -n 'var bridgeScriptPath = path.Join("/tmp", bridgeScriptName)\n\n' >> $@
	echo -n "const bridgeScriptPython = \`" >> $@
	cat add-juju-bridge.py >> $@
	echo -n '`\n' >> $@

format:
	pyfmt -i add-juju-bridge.py

clean:
	$(RM) bridgescript.go
