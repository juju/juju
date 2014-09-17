test:
	python -m unittest discover -vv ./tests -p '*.py'
lint:
	flake8 --max-line-length=80 --exclude=azure-sdk-for-python-master .
