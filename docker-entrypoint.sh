#!/bin/sh
set -e

# If the first argument is "migrate", run the migration tool.
if [ "$1" = 'migrate' ]; then
    # Remove the 'migrate' argument from the list
    shift
    # Run the migrate binary with the rest of the arguments (e.g., "up")
    exec migrate -path /migrations -database "$DB_SOURCE" "$@"
fi

# Otherwise, execute the command that was passed to this script.
# By default, this will be "/app/server".
exec "$@"