import os
import sys
import yaml
from pydiscourse import DiscourseClient
from pydiscourse.exceptions import DiscourseClientError


# Get configuration from environment variables
DISCOURSE_HOST = os.environ.get('DISCOURSE_HOST', 'https://discourse.charmhub.io/')
DISCOURSE_API_USERNAME = os.environ.get('DISCOURSE_API_USERNAME')
DISCOURSE_API_KEY = os.environ.get('DISCOURSE_API_KEY')
DOCS_DIR = os.environ.get('DOCS_DIR')
TOPIC_IDS = os.environ.get('TOPIC_IDS')

client = DiscourseClient(
    host=DISCOURSE_HOST,
    api_username=DISCOURSE_API_USERNAME,
    api_key=DISCOURSE_API_KEY,
)


def main():
    if len(sys.argv) < 1:
        sys.exit('no command provided, must be one of: check, sync, create, delete')

    command = sys.argv[1]
    if command == 'check':
        check()
    elif command == 'sync':
        sync()
    elif command == 'create':
        create()
    elif command == 'delete':
        delete()
    else:
        exit(f'unknown command "{command}"')


def check():
    """
    Check all docs in the DOCS_DIR have a corresponding entry in the TOPIC_IDS
    file, and that the corresponding topic exists on Discourse.
    """
    topic_ids = get_topic_ids()
    no_topic_id = []
    no_discourse_topic = []

    for entry in os.scandir(DOCS_DIR):
        if not is_markdown_file(entry):
            print(f'entry {entry.name}: not a Markdown file: skipping')
            continue

        doc_name = removesuffix(entry.name, ".md")

        if doc_name not in topic_ids:
            print(f'doc {doc_name}: no topic ID found')
            no_topic_id.append(doc_name)
            continue

        topic_id = topic_ids[doc_name]
        print(f'doc {doc_name} (topic #{topic_id}): checking topic on Discourse')
        try:
            client.topic(
                slug='',
                topic_id=topic_ids[doc_name],
            )
        except DiscourseClientError:
            print(f'doc {doc_name} (topic #{topic_id}): not found on Discourse')
            no_discourse_topic.append(doc_name)

    if no_topic_id:
        print(f"The following docs don't have corresponding entries in {TOPIC_IDS}.")
        print(f"Please create new Discourse topics for them, and add the new topic IDs to {TOPIC_IDS}.")
        for doc_name in no_topic_id:
            print(f' - {doc_name}')

    if no_discourse_topic:
        print("The following docs don't have corresponding topics on Discourse.")
        print(f"Please create new Discourse topics for them, and update the topic IDs in f{TOPIC_IDS}.")
        for doc_name in no_discourse_topic:
            print(f' - {doc_name} (topic #{topic_ids[doc_name]})')

    if no_topic_id or no_discourse_topic:
        sys.exit(1)


def sync():
    """
    Sync all docs in the DOCS_DIR with their corresponding topics on Discourse.
    """
    topic_ids = get_topic_ids()
    couldnt_sync = {}  # doc_name -> reason

    for entry in os.scandir(DOCS_DIR):
        if not is_markdown_file(entry):
            print(f'entry {entry.name}: not a Markdown file: skipping')
            continue

        doc_name = removesuffix(entry.name, ".md")
        content = open(entry.path, 'r').read()

        if doc_name not in topic_ids:
            couldnt_sync[doc_name] = 'no topic ID in yaml file'
            continue

        topic_id = topic_ids[doc_name]
        print(f'doc {doc_name} (topic #{topic_id}): checking for changes')
        try:
            # API call to get the post ID from the topic ID
            # TODO: we could save the post IDs in a separate yaml file and
            #   avoid this extra API call
            topic = client.topic(
                slug='',
                topic_id=topic_id,
            )
        except DiscourseClientError:
            couldnt_sync[doc_name] = f'no topic with ID #{topic_id} on Discourse'
            continue

        post_id = topic['post_stream']['posts'][0]['id']
        # Get current contents of post
        try:
            post2 = client.post_by_id(
                post_id=post_id
            )
        except DiscourseClientError as e:
            couldnt_sync[doc_name] = f"couldn't get post for topic ID #{topic_id}: {e}"
            continue

        current_contents = post2['raw']
        if current_contents == content.rstrip('\n'):
            print(f'doc {doc_name} (topic #{topic_ids[doc_name]}): already up-to-date: skipping')
            continue

        # Update Discourse post
        print(f'doc {doc_name} (topic #{topic_ids[doc_name]}): updating')
        try:
            client.update_post(
                post_id=post_id,
                content=content,
            )
        except DiscourseClientError as e:
            couldnt_sync[doc_name] = f"couldn't update post with ID #{post_id}: {e}"
            continue

    if len(couldnt_sync) > 0:
        print("Failed to sync the following docs:")
        for doc_name, reason in couldnt_sync.items():
            print(f' - {doc_name}: {reason}')
        sys.exit(1)


def create():
    """
    Create new Discourse topics for each doc name provided.
    """
    topic_ids = get_topic_ids()
    docs = sys.argv[2:]

    for doc_name in docs:
        if doc_name in topic_ids:
            print(f'skipping doc {doc_name}, it already has a topic ID')
            continue

        path = os.path.join(DOCS_DIR, doc_name+'.md')
        try:
            content = open(path, 'r').read()
        except OSError as e:
            print(f"couldn't open {path}: {e}")
            continue

        # Create new Discourse post
        print(f'creating new post for doc {doc_name}')
        post = client.create_post(
            title=post_title(doc_name),
            category_id=22,
            content=content,
            tags=['olm', 'autogenerated'],
        )
        new_topic_id = post['topic_id']
        print(f'doc {doc_name}: created new topic #{new_topic_id}')

        # Save topic ID in yaml map for later
        topic_ids[doc_name] = new_topic_id
        with open(TOPIC_IDS, 'w') as file:
            yaml.safe_dump(topic_ids, file)


def delete():
    """
    Delete all Discourse topics in the TOPIC_IDS file.
    """
    topic_ids = get_topic_ids()

    for doc_name, topic_id in topic_ids.items():
        print(f'deleting doc {doc_name} (topic #{topic_id})')
        client.delete_topic(
            topic_id=topic_id
        )

        # Update topic ID yaml map
        del topic_ids[doc_name]
        with open(TOPIC_IDS, 'w') as file:
            yaml.safe_dump(topic_ids, file)


def get_topic_ids():
    with open(TOPIC_IDS, 'r') as file:
        topic_ids = yaml.safe_load(file)
        return topic_ids or {}


def is_markdown_file(entry: os.DirEntry) -> bool:
    return entry.is_file() and entry.name.endswith(".md")


def removesuffix(text, suffix):
    if suffix and text.endswith(suffix):
        return text[:-len(suffix)]
    return text


def post_title(doc_name: str) -> str:
    if doc_name == 'index':
        return 'Juju CLI commands'
    return f"Command '{doc_name}'"


if __name__ == "__main__":
    main()
