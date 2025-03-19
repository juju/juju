#!/usr/bin/env bash

set -e

if [ "$#" -ne 1 ]; then
    echo "need one package to refactor"
fi

set -e

SCRIPT_DIR=$(dirname "$(realpath "$0")")

# Step 1: Run return.go over the package to refactor and make sure all errors
# being return out of function have a nil check around them.
go run scripts/errors-patch/return.go "${1}"

# Step 2: Run the go patch files over the target directory.
for patch_file in $(ls "$SCRIPT_DIR"/patches/*.patch | sort); do
  # Execute each .patch file (you can replace `patch` with the command to execute the patch)
  echo "running go patch with: $patch_file"
  gopatch -p "${patch_file}" "${1}/..."
done

# Step 4 fix up imports that have been modified by go patch.
gci  write --skip-generated -s standard -s default -s 'Prefix(github.com/juju/juju)' "${1}/."

# Step 5 remove unused imports that are going to blow up the compiler.
goimports -w "${1}/."

#  Step 6 run the post patch file that renames imports after everything is in a
    # state we can accept.
gopatch -p "${SCRIPT_DIR}/post.patch" "${1}/..."

# Step 7: Run sed replacement steps over go patched files.

# Step 7a: Fix up patch of errors that took err as the first argument.
# This sed step is fixing lines of the form:
# - errors.Errorf("some message" + ": %w", err) to errors.Errorf("some message %w", err)
# We do this because go patch doesn't have the ability to modify strings.
find "$1" -type f -iname '*.go' -exec sed -i '' -E "s,\"(.*)\"[ ]?\+[ ]?\"\: \%w\",\"\1\: \%w\",g" "{}" +;
#
# This sed step is fixing lines of the form:
# - errors.Errorf("some message" + " %w", err) to errors.Errorf("some message %w", err)
# We do this because go patch doesn't have the ability to modify strings.
find "$1" -type f -iname '*.go' -exec sed -i '' -E "s,\"(.*)\"[ ]?\+[ ]?\"\ \%w\",\"\1\ \%w\",g" "{}" +;

# Step 7b: Remove %w for errors that were using errors.Hide
# This sed step is fixing lines of the form:
# - errors.Errorf("some message%w").Add(someerror) to errors.Errorf("some message").Add(someerror)
# We do this because go patch doesn't have the ability to modify strings. We
# have ended up with these forms as errors.Hide gets removed.
find "$1" -type f -iname '*.go' -exec sed -i '' -E "s,\"(.*[^ ])\%w\",\"\1\",g" "{}" +;
