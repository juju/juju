import os
import subprocess
import shutil

# Remove existing cli folder to regenerate it
if os.path.exists("cli"):
    shutil.rmtree("cli")

# Generate the docs using "juju documentation" command.
os.makedirs('.sphinx/deps/manpages', exist_ok=True)
subprocess.run(["/home/aflynn/go/bin/juju", 'documentation', '--split', '--no-index', '--out', 'cli'],
                   check=True)

titles = []

for page in [x for x in os.listdir('cli')]:
    title = page[:-3]
    # Add sphinx names to each file.
    with open(os.path.join('cli/', page), 'r+') as mdfile:
        content = mdfile.read()
        # Remove trailing seperated (e.g. ----)
        content = content.rstrip(" -\n")
        mdfile.seek(0, 0)
        mdfile.write('(' + page + ')=\n' + 
                     '# `' + title + '`\n' + 
                     content)
    titles.append(title)
    

# Add the template for the index file containing the command list.
if (not os.path.isfile('cli/index.md')):
    os.system('cp ' + 'cli_index.template ' + 'cli/index.md')

# Write the list of commands to the index file toctree.
with open("cli/index.md", 'a') as index:
    index.write('n' + '')
    titles.sort()
    for title in titles:
        index.write(title + '\n')
    index.write("```")
