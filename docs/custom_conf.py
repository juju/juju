import os
import shutil
import subprocess

###################
# Generate CLI docs
###################

cli_dir = "user/reference/juju-cli/list-of-juju-cli-commands/"

# Remove existing cli folder to regenerate it
if os.path.exists(cli_dir):
    shutil.rmtree(cli_dir)

# Generate the CLI docs using "juju documentation" command.
subprocess.run(["juju", 'documentation', '--split', '--no-index', '--out', cli_dir],
                   check=True)

for page in os.listdir(cli_dir):
    title = "`juju " + page[:-3]+ "`"
    anchor = "command-juju-" + page[:-3]
    # Add sphinx names to each file.
    with open(os.path.join(cli_dir, page), 'r+') as mdfile:
        content = mdfile.read()
        # Remove trailing seperated (e.g. ----)
        content = content.rstrip(" -\n")
        mdfile.seek(0, 0)
        mdfile.write('(' + anchor + ')=\n' +
                     '# ' + title + '\n' +
                     content)


# Add the template for the index file containing the command list.
if (not os.path.isfile(cli_dir + 'index.md')):
    os.system('cp ' + 'user/reference/juju-cli/cli_index.template ' + cli_dir + 'index.md')

#################################
# Generate controller config docs
#################################


