(testing)=
# Testing

New or updated `juju` code will not pass review unless there are tests associated with the code. For code additions, the
tests should cover as much of the new code as practical; for code changes, the tests should be updated, or it should be
shown that the existing tests suffice. Either way, before requesting a review, you need to show that there is already
test coverage and that the new code / refactoring didn't break anything.


```{toctree}
:titlesonly:
:glob:
:maxdepth: 1

testing/integration-testing/index
testing/unit-testing/index
```