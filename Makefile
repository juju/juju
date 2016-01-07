p="*.py"
test:
	TMPDIR=/tmp python -m unittest discover -vv ./tests -p "$(p)"
lint:
	flake8 --max-line-length=80 $$(find . -name '*.py')
cover:
	python -m coverage run --source="./" --omit "./tests/*" -m unittest discover -vv ./tests
	python -m coverage report
clean:
	find . -name '*.pyc' -delete
.PHONY: lint test cover clean
