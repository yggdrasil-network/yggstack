#!/bin/sh

# Get the current branch name
BRANCH="$GITHUB_REF_NAME"
if [ -z "$BRANCH" ]; then
  BRANCH=$(git symbolic-ref --short HEAD 2>/dev/null)
fi

if [ $? != 0 ] || [ -z "$BRANCH" ]; then
  printf "yggstack"
  exit 0
fi

# Remove "/" characters from the branch name if present
BRANCH=$(echo $BRANCH | tr -d "/")

# Check if the branch name is not develop
if [ "$BRANCH" = "develop" ]; then
  printf "yggstack"
  exit 0
fi

# If it is something other than develop, append it
printf "yggstack-%s" "$BRANCH"
