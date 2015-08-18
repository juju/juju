test:
	TMPDIR=/tmp python -m unittest discover -vv ./tests -p '*.py'
lint:
	flake8 --max-line-length=80 .
clean:
	find . -name '*.pyc' -delete
