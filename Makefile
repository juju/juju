BASE=$(shell dirname $(CURDIR))
test:
	HOME=$(BASE) python -m unittest discover -vv . -p '*.py'
lint:
	pyflakes .
