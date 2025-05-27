#!/bin/bash

# Script to find Go compiler errors in a workspace and dump them to a file.

# --- Configuration ---
WORKSPACE_ROOT="${1:-.}" # Default to current directory if no argument is provided
OUTPUT_FILE="go_compiler_errors.log"
TMP_ERROR_FILE=$(mktemp) # Temporary file to hold errors for a single Go file

# --- Helper Functions ---
cleanup() {
    rm -f "$TMP_ERROR_FILE"
}
trap cleanup EXIT

# --- Main Logic ---

# Clear/create the output file
> "$OUTPUT_FILE"

echo "Scanning for Go files in '$WORKSPACE_ROOT'..."
echo "Errors will be logged to '$OUTPUT_FILE'"
echo "--------------------------------------"

# Find all .go files, excluding vendor, .git, and other common non-source dirs.
# -print0 and read -d $'\0' handle filenames with spaces or special characters.
find "$WORKSPACE_ROOT" -type d \( -name 'vendor' -o -name '.git' -o -name 'node_modules' -o -name 'dist' -o -name 'build' \) -prune -o -type f -name "*.go" -print0 | \
while IFS= read -r -d $'\0' go_file; do
    echo "Checking: $go_file"

    # Attempt to compile the file. Errors go to stderr.
    # We redirect stderr to our temporary file.
    # Using `go build` on a single file works best if it's in a package context.
    # -gcflags="-e" attempts to show more errors rather than stopping at the first.
    # `go build` on a single file which is not 'package main' might sometimes report
    # "no main package found", but it often still type-checks it within its package.
    # If the file is part of a package, `go build` from the package directory (`cd $(dirname "$go_file") && go build .`)
    # would be more robust for package-level errors, but the request is per-file.
    # Let's try `go build` on the file directly.
    
    current_file_had_errors=false
    error_output_for_file=""
    error_count_for_file=0

    # Run go build and capture its stderr. The exit status of go build might not always be non-zero
    # for type errors if it's just checking a single non-main file, so we rely on stderr.
    # We use a subshell to cd into the directory of the go_file to provide package context.
    # This makes `go build .` check the package the file belongs to.
    # The error messages will contain file paths relative to this directory or absolute.
    pkg_dir=$(dirname "$go_file")
    file_basename=$(basename "$go_file")

    # (cd "$pkg_dir" && go build -gcflags="-e" -o /dev/null "$file_basename" 2> "$TMP_ERROR_FILE")
    # Simpler: go build can take the full path to the .go file
    # It will infer the package from the file's location and content.
    go build -gcflags="-e" -o /dev/null "$go_file" 2> "$TMP_ERROR_FILE"
    # Alternative using `go tool compile` which is more for single files but less package-aware:
    # go tool compile -e "$go_file" 2> "$TMP_ERROR_FILE" # -e means print errors to stderr

    if [ -s "$TMP_ERROR_FILE" ]; then # If TMP_ERROR_FILE is not empty
        # Filter out lines that are just "build output is /dev/null" or empty, or start with # (package info)
        # and see if anything substantial remains.
        # Go error format: path/to/file.go:line:col: message
        
        # Read errors from the temp file
        while IFS= read -r line; do
            # Match the typical Go error line: file:line:column: message
            if [[ "$line" =~ ^([^:]+):([0-9]+):([0-9]+):[[:space:]]*(.*) ]]; then
                err_file_path_from_compiler="${BASH_REMATCH[1]}"
                err_line_num="${BASH_REMATCH[2]}"
                # err_col="${BASH_REMATCH[3]}" # Not used in the requested format
                err_message="${BASH_REMATCH[4]}"

                # The file path from the compiler could be relative or absolute.
                # If it's relative, it's usually relative to where `go build` was conceptually run.
                # If we ran `go build path/to/file.go`, err_file_path_from_compiler IS path/to/file.go.
                
                # Only process errors that seem to originate from the file we are currently checking
                # or if it's an absolute path that matches.
                # This is a heuristic because an error in file A could be *caused* by file B (e.g. bad import).
                # For simplicity, we focus on errors reported *for* the file itself.
                # A more robust check would be `[[ "$(realpath "$err_file_path_from_compiler")" == "$(realpath "$go_file")" ]]`
                # but realpath might not be available or might behave differently.
                # Let's assume the compiler error path is the one we need for `sed`.

                if ! $current_file_had_errors; then
                    error_output_for_file+="${go_file}\n"
                    current_file_had_errors=true
                fi
                
                ((error_count_for_file++))
                error_output_for_file+="error ${error_count_for_file}:\n"
                error_output_for_file+="${err_message}\n"

                # Attempt to get the actual line of code from the source file
                # The $err_file_path_from_compiler is the most reliable source for `sed`.
                code_line=$(sed -n "${err_line_num}p" "$err_file_path_from_compiler" 2>/dev/null)
                if [ -n "$code_line" ]; then
                    error_output_for_file+="${code_line}\n"
                else
                    error_output_for_file+="[Could not retrieve source line ${err_line_num} from '${err_file_path_from_compiler}']\n"
                fi
                error_output_for_file+="\n" # Blank line after each error block as per example format

            elif [[ -n "$line" && ! "$line" =~ ^# && ! "$line" =~ build\ output\ is && ! "$line" =~ ^exit\ status ]]; then
                # This might be a continuation of a multi-line error message or a general error
                # not fitting the file:line:col pattern. Append it if we've started error reporting.
                # The user's format is specific, so this might not be desired.
                # For now, we only process structured file:line:col errors.
                # To be cautious, let's log these "unstructured" lines too if they appear.
                if ! $current_file_had_errors; then
                    error_output_for_file+="${go_file}\n" # Header for the file
                    error_output_for_file+="general error output (may not be compiler error):\n"
                    current_file_had_errors=true
                fi
                error_output_for_file+="${line}\n"
            fi
        done < "$TMP_ERROR_FILE"

        if $current_file_had_errors; then
            printf "%b" "$error_output_for_file" >> "$OUTPUT_FILE"
            # Add an extra newline between files if there were errors
            echo "" >> "$OUTPUT_FILE"
        fi
    fi
    # Clear temp file for next iteration
    > "$TMP_ERROR_FILE"
done

echo "--------------------------------------"
echo "Error checking complete."
echo "Results logged to '$OUTPUT_FILE'"

# Note: To see if any errors were logged, you can check:
# if [ -s "$OUTPUT_FILE" ]; then echo "Errors found."; else echo "No errors found."; fi