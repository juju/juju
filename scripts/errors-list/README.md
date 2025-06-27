# Juju Errors List Generator

## Introduction

The Juju codebase defines numerous error constants across different domain packages. As the codebase grows, it becomes increasingly difficult for developers to:

1. Know what error constants already exist
2. Choose the appropriate error constant for a given situation
3. Maintain consistency in error handling across the codebase

This script addresses these challenges by generating a comprehensive, searchable list of all error constants defined in the Juju codebase, making it easier to discover and reuse existing error definitions.

## Features

The errors-list generator:

- Scans all `./domain/**/errors/*.go` files for error constants
- Extracts error names, domains, and documentation comments
- Generates a markdown table with all errors, their domains, and documentation
- Provides links to the source files where each error is defined
- Supports sorting errors alphabetically or by domain

## Usage

Run the script from the root of the Juju project:

```bash
go run scripts/errors-list/main.go
```

This will generate an `errors-list.md` file in the current directory containing a table of all error constants.

### Specifying Project Directory

You can specify a different project directory:

```bash
go run scripts/errors-list/main.go /path/to/juju
```

### Sorting Options

By default, errors are sorted alphabetically by name. You can change the sorting with the `--sort` flag:

```bash
# Sort alphabetically (default)
go run scripts/errors-list/main.go --sort alph

# Sort by domain
go run scripts/errors-list/main.go --sort domain
```

## Output

The generated markdown file contains a table with the following columns:

- **Error**: The name of the error constant
- **Domain**: The domain package where the error is defined (with a link to the source file)
- **Documentation**: The documentation comment for the error

Example:

```markdown
| Error | Domain | Documentation |
| ---- | ---- | ---- |
| ApplicationNotFound | [application](domain/application/errors/errors.go) | application not found |
| EndpointNotFound | [application](domain/application/errors/errors.go) | endpoint not found |
```

## Benefits

- **Discoverability**: Easily find existing error constants
- **Consistency**: Promote reuse of existing errors instead of creating duplicates
- **Documentation**: Access error documentation in a single place
- **Maintainability**: Understand the error landscape across the codebase