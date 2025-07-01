#!/bin/sh
set -e

# This script acts as a router.
# It checks the first argument ($1) to decide what to do.

# If the first argument is "migrate", it runs the migration tool.
# It then passes along any other arguments (like "up" or "down") to the migrate command.
if [ "$1" = 'migrate' ]; then
    # The 'shift' command removes the first argument ("migrate") from the list.
    shift
    # Now, run the actual migrate binary with the remaining arguments ("$@").
    # The DB_SOURCE variable is passed in from the .env file.
    exec migrate -path /migrations -database "$DB_SOURCE" "$@"
fi

# If the first argument is not "migrate", it assumes you want to run the web server.
# The 'exec "$@"' command runs whatever command was passed to the script.
# In our setup, this will be "/app/server" by default.
exec "$@"