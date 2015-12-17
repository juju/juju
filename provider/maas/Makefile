all: bridgescript.go

bridgescript.go: add-juju-bridge.py Makefile
	$(RM) $@
	echo -n '// This file is auto generated. Edits will be lost.\n\n' >> $@
	echo -n 'package maas\n\n' >> $@
	echo -n '//go:generate make -q\n\n' >> $@
	echo -n "const bridgeScriptPythonBashDef = \`python_script=\$$(cat <<'PYTHON_SCRIPT'\n" >> $@
	cat add-juju-bridge.py >> $@
	echo -n 'PYTHON_SCRIPT\n)`\n' >> $@

format:
	pyfmt -i add-juju-bridge.py

clean:
	$(RM) bridgescript.go
