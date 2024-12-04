# Introduction
This folder contains scripts and resources for converting Juju packages from
using github.com/juju/errors to using github.com/juju/juju/internal/errors.

# Usage
The scripts expect to be run from the root of the project. Example refactor of
ever go file under the domain package:
```bash
./scripts/errors-patch/run.sh domain
```

# Notes
These scripts are not perfect and cannot fix every variation of error generation
that exists. Some manual refactoring can be expected.