import os
import shutil
import subprocess

def generate_cli_docs():
    cli_dir = "user/reference/juju-cli/"
    generated_cli_docs_dir = cli_dir + "list-of-juju-cli-commands/"
    cli_index_template = cli_dir + 'cli_index.template'

    # Remove existing cli folder to regenerate it
    if os.path.exists(generated_cli_docs_dir):
        shutil.rmtree(generated_cli_docs_dir)

    # Generate the CLI docs using "juju documentation" command.
    subprocess.run(['juju', 'documentation', '--split', '--no-index', '--out', generated_cli_docs_dir],
                       check=True)

    for page in os.listdir(generated_cli_docs_dir):
        title = "`juju " + page[:-3]+ "`"
        anchor = "command-juju-" + page[:-3]
        # Add sphinx names to each file.
        with open(os.path.join(generated_cli_docs_dir, page), 'r+') as mdfile:
            content = mdfile.read()
            # Remove trailing seperated (e.g. ----)
            content = content.rstrip(" -\n")
            mdfile.seek(0, 0)
            mdfile.write('(' + anchor + ')=\n' +
                         '# ' + title + '\n' +
                         content)

    # Add in the index file containing the command list.
    subprocess.run(['cp', cli_index_template, generated_cli_docs_dir + 'index.md'])


    result = subprocess.run(['juju', 'version'], capture_output=True, text=True)
    print("generated cli command docs using juju verison found in path: " + result.stdout.rstrip())

#################################
# Generate controller config docs
#################################


