test:
	python -m unittest discover -vv . -p '*.py'
lint:
	flake8 .
