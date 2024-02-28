# Actions charm

A charm used in the actions integration tests. It has two actions:
- `fortune` which requires a length (length="short" or "long") and returns your
  fortune , see charmcraft.yaml for more.
- `list-my-params` which takes arbitrary key value parameters and returns them
  in the action result.
