test:
	python -m unittest discover -vv ./tests -p '*.py'
lint:
	flake8 .
