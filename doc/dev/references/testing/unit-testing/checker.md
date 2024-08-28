> *[Testing](../index.md) > [Unit testing](index.md) > Checker*

A **checker** is a core concept of `gocheck` and will feel familiar to anyone who has used the python
testtools.Assertions are made on the gocheck.C methods.

```go

c.Check(err, jc.ErrorIsNil)

c.Assert(something, gc.Equals, somethingElse)

```

The `Check` method will cause the test to fail if the checker returns false, but it will continue immediately, causing
the test to fail and will continue with the test.`Assert` if it fails will cause the test to stop immediately.

For further discussion, we have the following parts:

`c.Assert(observed, checker, args...)`

The key checkers in the `gocheck` module that juju uses most frequently are:

|                |                                                                                                                                                                                      |
|----------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `IsNil`        | The observed value must be `nil`.                                                                                                                                                    |
| `NotNil`       | The observed value must not be `nil`.                                                                                                                                                |
| `Equals`       | The observed value must be the same type and value as the arg, which is the expected value.                                                                                          |
| `DeepEquals`   | Checks for equality for more complex types like slices, maps, or structures.This is DEPRECATED in favor of the DeepEquals from the `github.com/juju/testing/checkers` covered below. |
| `ErrorMatches` | The observed value is expected to be an `error`, and the arg is a string that is a regular expression and used to match the error string.                                            |
| `Matches`      | A regular expression match where the observed value is a string.                                                                                                                     |
| `HasLen`       | The expected value is an integer, and works happily on nil slices or maps.                                                                                                           |

In the `juju` project various patterns of testing have emerged over time. These have since been encoded into new and
increasingly more sophisticated checkers.These are found in `github.com/juju/testing/checkers`, and are usually imported
with the alias `jc`.

The matchers there include (not an exclusive list):

|                  |                                                                                                                                                              |
|------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `IsTrue`         | Just an easier way to say `gc.Equals, true`.                                                                                                                 |
| `IsFalse`        | Observed value must be false.                                                                                                                                |
| `GreaterThan`    | For integer or float types.                                                                                                                                  |
| `LessThan`       | For integer or float types.                                                                                                                                  |
| `HasPrefix`      | Obtained is expected to be a string or a `Stringer`, and the string (or string value) must have the arg as the start of the string.                          |
| `HasSuffix`      | The same as `HasPrefix` but checks the end of the string.                                                                                                    |
| `Contains`       | Obtained is a string or `Stringer` and expected needs to be a string.The checker passes if the expected string is a substring of the obtained value.         |
| `DeepEquals`     | Works the same way as the `gocheck.DeepEquals` except gives better errors when the values do not match.                                                      |
| `SameContents`   | Obtained and expected are slices of the same type, the checker makes sure that the values in one are in the other.They do not have the be in the same order. |
| `Satisfies`      | The arg is expected to be `func(observed) bool` often used for error type checks.                                                                            |
| `IsNonEmptyFile` | Obtained is a string or `Stringer` and refers to a path.The checker passes if the file exists, is a file, and is not empty.                                  |
| `IsDirectory`    | Works similarly to `IsNonEmptyFile` but passes if the path element is a directory.                                                                           |
| `DoesNotExist`   | Also works with a string or `Stringer`, and passes if the path element does not exist.                                                                       |