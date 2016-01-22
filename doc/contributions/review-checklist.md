# Reviewing Checklist

A list of common (not exhaustive) mistakes to check for when reviewing. For further details please consult the [Style Guide](/doc/contributions/style-guide.md).


## General

- A PR needs a justification. This could be: 
    + An issue, ie “Fixes LP 12349000”.
    + A description justifying the change.
    + A link to a design document or mailing list discussion.
- Work out what the code is trying to fix (feature or bug).
    + Is it obvious that the code does this?
- Is this code valid for all operating systems?
- Are there any race conditions?


## Errors:

- Error messages are lowercase with no full stop (see 'Errors' in the [Style Guide](/doc/contributions/style-guide.md))
- All errors should be handled:
    + If not, a reason should be given in a comment
    + Search for `_`
    + Check signature of function calls that do not assign the result to a var.
- is juju/errors being used appropriately (see 'Errors' in the [Style Guide](/doc/contributions/style-guide.md))?


## Functions:

- Can the function be split into smaller functions and does this make the function clearer?
- Does inlining a func/var add clarity.
- Does the function return what it's signature says it will.
- Does the function have a helpful and sane name?
- Can you work out what the function parameters and results are, based on the signature? (named results where appropriate).
- Do all error paths release resources correctly (defer Close(), etc.).


## Tests:

- Is all functionality covered?
- Are all error cases covered?
- Function names describe what is being tested and the expected result (i.e. success or failure).
- Could similar tests be combined into a table based test?
- Should a table based test be broken out in more obvious tests? (this sometimes happens when there is too much logic or too many conditionals in the actual test block of the table based test).
- Is there repeated test boilerplate which should be extracted in functions on the testSuite?
- Is the appropriate base test suite used? (overuse of JujuConnSuite is common).
- Are the tests isolated from the environment? Could the test mock access to the API?
- Is the suite registered with Gocheck? (gc.Suite(...))
- Is setup and teardown done correctly? Do tearDown{Test,Suite} assume that their companion setUp{Test,Suite} functions completed successfully?
- Is external functionality being called that should be mocked instead?
- Each suite is set up directly before tests for that suite.
- Correct use of c.Check vs c.Assert.
- Temporary files must be created inside a path provided by c.Mkdir
- Variables from external modules should not be patched directly, (see 'Tests' in [Style Guide](/doc/contributions/style-guide.md)).
- Are test entities being created in the most efficient way? (e.g. using the test factory)?
- `doc/how-to-write-tests.txt` covers some basics of writing good tests.


## Comments:

- Function comments should start with the function name (see 'Comments' in the Style Guide)
- Check spelling and punctuation.
- Is the comment necessary and accurate (all comments become lies eventually)?
- Are all the public constants, variables, functions and methods documented?
- Is a comment needed?
- Todo convention e.g:

```
// TODO(axw) 2013-09-24 bug #1229507
// Description of todo
```


## Variables:
- Is a var only used once? If so, can we inline the var? e.g.

```go
func someFunc() string {
    a := "a string"
    return someOtherFunc(a)
}
```

can be simplified to:

```
func someFunc() string {
    return someOtherFunc("a string")
}
```

- Could/should a pointer be used instead of a reference?
- `var ( )` is not needed if it has only one var in it.


## Names:
- Is the var/type/func name self-describing? if not, can it be made so the name makes sense at the package scope (e.g. looking at godoc)?


## API:
- Ensure client makes a batch request (see the [API doc](../api.txt))


## Layout:
- Imports are grouped into 3 sections (see 'Layout' in [Style Guide](/doc/contributions/style-guide.md))
- Helper functions go below the functions they are helping


## Helper Functions

Ensure wheels are not being reinvented.

- Strings package for string manipulation.
- Testing factory for mock users, machines, units and relations.






