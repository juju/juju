import os
import subprocess
import shutil

cli_dir = "user/cli/"

# Remove existing cli folder to regenerate it
if os.path.exists(cli_dir):
    shutil.rmtree(cli_dir)

# Generate the docs using "juju documentation" command.
subprocess.run(["juju", 'documentation', '--split', '--no-index', '--out', cli_dir], check=True)

titles = []

for page in os.listdir(cli_dir):
    title = page[:-3]
    # Add sphinx names to each file.
    with open(os.path.join(cli_dir, page), 'r+') as mdfile:
        content = mdfile.read()
        # Remove trailing seperated (e.g. ----)
        content = content.rstrip(" -\n")
        mdfile.seek(0, 0)
        mdfile.write('(' + page + ')=\n' + 
                     '# `' + title + '`\n' + 
                     content)
    titles.append(title)
    

# Add the template for the index file containing the command list.
if (not os.path.isfile(cli_dir + 'index.md')):
    os.system('cp ' + 'user/cli_index.template ' + cli_dir + 'index.md')

# Write the list of commands to the index file toctree.
with open(cli_dir + "index.md", 'a') as index:
    titles.sort()
    for title in titles:
        index.write(title + '\n')
    index.write("```")
