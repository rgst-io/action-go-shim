#!/usr/bin/env bash
#
# Release the current action.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

NEW_VERSION="${1:-}"

# show_usage prints out the usage information for this script.
show_usage() {
	echo "scripts/release.sh <version>"
}

if [[ -z "$NEW_VERSION" ]]; then
	show_usage >&2
	exit 1
fi

# Ensure we're on the main branch
current_branch=$(git rev-parse --abbrev-ref HEAD)
current_commit=$(git rev-parse HEAD)
if [[ "$current_branch" != "main" ]]; then
	echo "Error: Must be on main branch to release (currently on $current_branch)" >&2
	exit 1
fi

NEW_VERSION="${NEW_VERSION#v}"

# Validate semver format and extract components
if ! [[ "$NEW_VERSION" =~ ^v?([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
	echo "Error: Version must be in semver format (e.g., 1.2.3 or v1.2.3)" >&2
	exit 1
fi

major="${BASH_REMATCH[1]}"
minor="$major.${BASH_REMATCH[2]}"
patch="$minor.${BASH_REMATCH[3]}"

major_tag="v$major"
minor_tag="v$minor"
patch_tag="v$patch"

echo "Releasing $patch_tag ($major_tag/$minor_tag)"
git tag "$patch_tag"
goreleaser --release-notes CHANGELOG.md --clean --snapshot
git tag -d "$patch_tag"

echo " -> Creating shim release with binaries"

tmpDir=$(mktemp -d)

cp -v "$REPO_ROOT/shim/shim.js" "$tmpDir"
cp -v "$REPO_ROOT/action.yml" "$tmpDir"
cp -v "$REPO_ROOT/dist/"{artifacts.json,checksums.txt,config.yaml,metadata.json} "$tmpDir/"
echo "$current_commit" >"$tmpDir/COMMIT"

# Copy over each artifact's files in a predictable naming scheme for the
# shim to execute.
readarray -t artifacts < <(jq -r '.[] | select(.type == "Binary") | .path+"|"+.extra.Binary+"-"+.goos+"-"+.goarch+(.extra.Ext // empty)' "$REPO_ROOT/dist/artifacts.json")
for artifact in "${artifacts[@]}"; do
	path=$(awk -F '|' '{ print $1 }' <<<"$artifact")
	output=$(awk -F '|' '{ print $2 }' <<<"$artifact")

	cp -v "$REPO_ROOT/$path" "$tmpDir/$output"
done

pushd "$tmpDir" >/dev/null
git init
git remote add origin https://github.com/rgst-io/action-go-shim
git switch -c release-base
git fetch origin main

git add -A .
git commit -am "chore: release $patch_tag"

tags=("$patch_tag" "$minor_tag" "$major_tag")
for tag in "${tags[@]}"; do
	git tag "$tag"

	# Only allow force pushing for major/minor. Patch we shouldn't allow
	# to be overwritten (at least not automatically).
	extra_args=("--force")
	if [[ "$tag" == "$patch_tag" ]]; then
		extra_args=()
	fi

	git push origin "${extra_args[@]}" "refs/tags/$tag"
done

gh release create --notes-from-tag "$patch_tag" ./*
popd >/dev/null

echo "Cleaning up temporary repository"
rm -rf "$tmpDir"
