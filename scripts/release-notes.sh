#!/bin/sh
# Print the CHANGELOG.md section of a version without its heading.
#
#   scripts/release-notes.sh 0.1.0
#
# "0.1" also matches a "## [0.1.0]" section. Prints nothing if the version
# has no section. Leading/trailing blank lines and Markdown link reference
# definitions ("[0.1.0]: https://...") are left out.
set -eu

version="${1:?usage: release-notes.sh <version>}"
changelog="${CHANGELOG:-CHANGELOG.md}"

awk -v v="$version" '
	/^## \[/ {
		if (found) exit
		h = $0
		sub(/^## \[/, "", h)
		sub(/\].*/, "", h)
		if (h == v || h == v ".0") found = 1
		next
	}
	/^\[[^]]+\]: / { next }
	found {
		if (NF == 0 && n == 0) next
		lines[++n] = $0
	}
	END {
		while (n > 0 && lines[n] ~ /^[ \t]*$/) n--
		for (i = 1; i <= n; i++) print lines[i]
	}
' "$changelog"
