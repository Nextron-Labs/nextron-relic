#!/bin/sh
# Publish the GitHub release of a tag with the given assets (used by
# .github/workflows/release.yml; needs gh and GH_TOKEN).
#
#   scripts/publish-release.sh v0.1.0 dist/*
#
# - Existing release or draft for the tag: assets are uploaded (replacing
#   files of the same name), title and notes are kept, a draft is published.
# - No release yet: it is created with the version's CHANGELOG.md section as
#   notes, or GitHub's generated notes if the changelog has none.
# - Tags with a pre-release suffix (v1.2.0-rc.1) become pre-releases.
set -eu

tag="${1:?usage: publish-release.sh <tag> <asset>...}"
shift
[ "$#" -gt 0 ] || { echo "no assets given" >&2; exit 1; }
for f in "$@"; do
	[ -f "$f" ] || { echo "asset $f does not exist" >&2; exit 1; }
done

version="${tag#v}"
prerelease=false
case "$version" in *-*) prerelease=true ;; esac

if gh release view "$tag" --json isDraft >/dev/null 2>&1; then
	echo "Updating existing release $tag"
	gh release upload "$tag" "$@" --clobber
	gh release edit "$tag" --draft=false --prerelease="$prerelease"
else
	echo "Creating release $tag"
	notes_file="$(mktemp)"
	trap 'rm -f "$notes_file"' EXIT
	"$(dirname "$0")/release-notes.sh" "$version" > "$notes_file"
	set -- "$@" --title "Relic $version" --verify-tag --prerelease="$prerelease"
	if [ -s "$notes_file" ]; then
		gh release create "$tag" "$@" --notes-file "$notes_file"
	else
		echo "CHANGELOG.md has no section for $version; using generated notes"
		gh release create "$tag" "$@" --generate-notes
	fi
fi
gh release view "$tag" --json url,isDraft,isPrerelease,assets --jq '{url, isDraft, isPrerelease, assets: [.assets[].name]}'
