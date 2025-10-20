 git add -A
          if [[ -n $(git diff HEAD) ]]; then
            # Print the full diff for debugging purposes
            git diff HEAD
            echo "*****"
            echo "The following generated files have been modified:"
            git diff --name-status HEAD
            echo "Please regenerate these files and check in the changes."
            echo "*****"
            exit 1
          fi
