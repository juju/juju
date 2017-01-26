all: bridgescript.go

# TODO add bridgescript_doc.go that explains (in a package comment)
# what this script (add-juju-bridge.py) does, and why, and how to
# change it.

bridgescript.go: add-juju-bridge.py Makefile
	$(RM) $@
	echo -n '// This file is auto generated. Edits will be lost.\n\n' >> $@
	echo -n 'package network\n\n' >> $@
	echo -n "const BridgeScriptPythonContent = \`" >> $@
	cat add-juju-bridge.py >> $@
	echo -n '`\n' >> $@

format:
	pyfmt -i add-juju-bridge.py

clean:
	$(RM) bridgescript.go
