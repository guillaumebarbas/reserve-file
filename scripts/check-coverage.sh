#!/bin/sh
# Fails unless the given coverage profile covers 100% of the statements.
set -eu

profile="${1:?usage: check-coverage.sh <coverage profile>}"
total=$(go tool cover -func="$profile" | awk '/^total:/ { print $3 }')

echo "total coverage: $total"
if [ "$total" != "100.0%" ]; then
	echo "coverage must be 100.0%. Functions not fully covered:" >&2
	go tool cover -func="$profile" | grep -v '100.0%' >&2
	exit 1
fi
