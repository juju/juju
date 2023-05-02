# discourse-sync

This Python script syncs Markdown docs to Discourse.

## Commands

| Command                  | Description                                                                                                                                                   |
|--------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `check`                  | Check that all docs in `$DOCS_DIR` have a corresponding entry in the `$TOPIC_IDS` file, and that this topic exists on Discourse. To be run on a pull request. |
| `sync`                   | Sync all docs in `$DOCS_DIR` to Discourse. To be run on each commit to the main branch.                                                                       |
| `create <doc-names> ...` | Create new topics for the provided doc names, and update the `$TOPIC_IDS` file.                                                                               |
| `delete`                 | Delete all topics with IDs listed in the `$TOPIC_IDS` file.                                                                                                   |


## Configuration

The script can be configured using the following environment variables:

| Variable name            | Description                                                                                                                                                                |
|--------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `DISCOURSE_HOST`         | URL for the Discourse server to sync docs to. If not set, this defaults to `https://discourse.charmhub.io/`.                                                               |
| `DISCOURSE_API_USERNAME` | Username to use for Discourse API requests. Must be a user with access to the API key provided in `DISCOURSE_API_KEY`. Use your own Discourse username if running locally. |
| `DISCOURSE_API_KEY`      | [API key](https://meta.discourse.org/t/create-and-configure-an-api-key/230124) for accessing the Discourse server's API.                                                   |
| `DOCS_DIR`               | Path to a directory containing Markdown files to sync (i.e. the argument provided to the `--out` flag of `juju documentation`).                                            |
| `TOPIC_IDS`              | Path to a YAML file mapping each doc name to its topic ID on Discourse.                                                                                                    |


## Suggested usage

```bash
export DISCOURSE_API_USERNAME=[your-discourse-username]
export DISCOURSE_API_KEY=[api-key]
export DOCS_DIR=./docs
export TOPIC_IDS=./.github/discourse-topic-ids.yaml

juju documentation --split --out=$DOCS_DIR --discourse-ids=$TOPIC_IDS # --no-index 
python3 ./scripts/discourse-sync/main.py sync
```