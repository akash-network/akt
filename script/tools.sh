#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
SEMVER=$SCRIPT_DIR/semver.sh

gomod="$SCRIPT_DIR/../go.mod"

macos_deps=(
	"bash"
	"direnv"
	"pv"
	"lz4"
)

debian_deps=(
	"make"
	"build-essential"
	"direnv"
	"unzip"
	"wget"
	"curl"
	"npm"
	"jq"
	"coreutils"
)

is_command() {
	command -v "$1" >/dev/null
}

function get_gotoolchain() {
	local gotoolchain
	local goversion
	local local_goversion
	local toolfile

	toolfile=$gomod

	if [[ "$GOWORK" != "off" ]] && [ -f "$GOWORK" ]; then
		toolfile=$GOWORK
	fi

	gotoolchain=$(grep -E '^toolchain go[0-9]{1,}.[0-9]{1,}.[0-9]{1,}$' <"$toolfile" | cut -d ' ' -f 2 | tr -d '\n')
	goversion=$(grep -E '^go [0-9]{1,}.[0-9]{1,}(.[0-9]{1,})?$' <"$toolfile" | cut -d ' ' -f 2 | tr -d '\n')

	if [[ ${gotoolchain} == "" ]]; then
		gotoolchain=go$goversion
	fi

	if [[ ${gotoolchain} == "" ]]; then
		# determine go toolchain from go version in go.mod
		if which go >/dev/null 2>&1; then
			# shellcheck disable=SC2086
			local_goversion=$(env -i HOME="$HOME" GOTOOLCHAIN=local $SHELL -l -c "go version | cut -d ' ' -f 3 | sed 's/go*//' | tr -d '\n'")
			if ! [[ $($SEMVER compare "v$local_goversion" v"$goversion") -ge 0 ]]; then
				goversion=$local_goversion
			else
				local_goversion=
			fi
		fi

		if [[ "$local_goversion" == "" ]]; then
			goversion=$(curl -s "https://go.dev/dl/?mode=json&include=all" | jq -r --arg regexp "^go$goversion" '.[] | select(.stable == true) | select(.version | match($regexp)) | .version' | head -n 1 | sed -e s/^go//)
		fi

		if [[ $goversion != "" ]] && [[ $($SEMVER compare "v$goversion" v1.21.0) -ge 0 ]]; then
			gotoolchain=go${goversion}
		else
			gotoolchain=go$(grep -E '^go [0-9]{1,}.[0-9]{1,}$' <"$toolfile" | cut -d ' ' -f 2 | tr -d '\n').0
		fi
	fi

	echo -n "$gotoolchain"
}

replace_paths() {
	local file="${1}"
	local cimport="${2}"
	local nimport="${3}"
	local sedcmd=sed

	if [[ "$OSTYPE" == "darwin"* ]]; then
		sedcmd=gsed
	fi

	$sedcmd -ri "s~$cimport~$nimport~" "${file}"
}

function replace_import_path() {
	local next_major_version=$1
	local curr_module_name
	local curr_version
	local new_module_name

	curr_module_name=$(go list -m)
	curr_version=$(echo "$curr_module_name" | sed -n 's/.*v\([0-9]*\).*/\1/p')
	new_module_name=${curr_module_name%/"v$curr_version"}/$next_major_version

	echo "current import paths are $curr_module_name, replacing with $new_module_name"

	declare -a modules_to_upgrade_manually

	modules_to_upgrade_manually+=("./go/go.mod")

	echo "preparing files to replace"

	declare -a files

	while IFS= read -r line; do
		files+=("$line")
	done < <(find . -type f -not \( \
		-path "./install.sh" \
		-or -path "./upgrades/*" \
		-or -path "./.cache/*" \
		-or -path "./dist/*" \
		-or -path "./.git*" \
		-or -name "*.md" \
		-or -path "./.idea/*" \))

	echo "updating all files"

	for file in "${files[@]}"; do
		if test -f "$file"; then
			# skip files that need manual upgrading
			for excluded_file in "${modules_to_upgrade_manually[@]}"; do
				if [[ "$file" == *"$excluded_file"* ]]; then
					continue 2
				fi
			done

			replace_paths "$file" "\"$curr_module_name" "\"$new_module_name"
		fi
	done

	echo "updating go.mod"
	for retract in $(go mod edit --json | jq -cr '.Retract | if . != null then .[] else empty end'); do
		local low
		local high

		low=$(jq -r '.Low' <<<"$retract")
		high=$(jq -r '.High' <<<"$retract")
		echo "    dropping retract: [$low, $high]"
		go mod edit -dropretract=["$low","$high"]
	done

	replace_paths "./go.mod" "$curr_module_name" "$new_module_name"
}

function install_gha_deps() {
	if [[ "$OSTYPE" == "darwin"* ]]; then
		echo "Detected Darwin based system"

		if ! is_command brew; then
			echo "homebrew is not installed. visit https://brew.sh"
			exit 1
		fi

		local tools

		if ! is_command make || [[ $(make --version | head -1 | cut -d" " -f3 | cut -d"." -f1) -lt 4 ]]; then
			tools="$tools make"
		fi

		# shellcheck disable=SC2068
		for dep in ${macos_deps[@]}; do
			echo -n "detecting $dep ..."
			status="(installed)"
			if ! brew list "$dep" >/dev/null 2>&1; then
				tools="$tools $dep"
				status="(not installed)"
			fi

			echo " $status"
		done

		if [[ "$tools" != "" ]]; then
			# don't put quotes around $tools!
			# shellcheck disable=SC2086
			brew install $tools
		else
			echo "All requirements already met. Nothing to install"
		fi
	elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
		if is_command dpkg; then
			echo "Detected Debian based system"
			local tools

			# shellcheck disable=SC2068
			for dep in ${debian_deps[@]}; do
				echo -n "detecting $dep ..."
				status="(installed)"
				if ! dpkg -l "$dep" >/dev/null 2>&1; then
					tools="$tools $dep"
					status="(not installed)"
				fi
				echo " $status"
			done

			cmd="apt-get"

			if is_command sudo; then
				cmd="sudo $cmd"
			fi

			if [[ "$tools" != "" ]]; then
				$cmd update
				# don't put quotes around $tools!
				# shellcheck disable=SC2086
				(
					set -x
					$cmd install -y $tools
				)
			else
				echo "All requirements already met. Nothing to install"
			fi
		fi
	else
		echo "Unsupported OS $OSTYPE"
		exit 1
	fi
}

release_log() {
	printf '==> %s\n' "$*"
}

release_die() {
	printf 'tag-release: %s\n' "$*" >&2
	exit 1
}

# Prints the highest semantic version among the tag names on stdin, or
# nothing when there is none. Callers pipe `git tag --list` in so pipefail
# catches a git failure; errexit does not reach inside $(...).
release_latest_tag() {
	local t
	local latest=""

	while IFS= read -r t; do
		[[ "$("$SEMVER" validate "$t")" == valid ]] || continue
		if [[ -z "$latest" ]] || [[ "$("$SEMVER" compare "$t" "$latest")" == 1 ]]; then
			latest=$t
		fi
	done

	printf '%s' "$latest"
}

# EXIT trap while release_prepare has the checkout on the release branch.
release_stopped_hint() {
	local branch=$1
	local start=$2

	printf 'tag-release: stopped on %s. To start over: git switch %s && git branch -D %s, plus git push origin --delete %s if it was pushed.\n' \
		"$branch" "$start" "$branch" "$branch" >&2
}

# The release_* helpers below run inside tag_release and share its locals
# (remote, base, repo, tag, branch, pr, sha) through bash's dynamic scope.

# Fails unless every fetch and push URL configured for $remote names $repo,
# the repository gh opens the PR in, so the branch and tag cannot land in a
# fork. Reads the configured URLs: insteadOf rewrites are transport details.
release_require_canonical_remote() {
	local urls
	local url
	local slug
	local want

	urls=$(git config --get-all "remote.$remote.url") ||
		release_die "no $remote remote is configured"
	# A remote without a pushurl pushes to its url.
	urls+=$'\n'$(git config --get-all "remote.$remote.pushurl" || true)
	want=$(tr '[:upper:]' '[:lower:]' <<<"$repo")

	while IFS= read -r url; do
		[[ -n "$url" ]] || continue
		slug=$(sed -E 's#^(git@github\.com:|ssh://git@github\.com/|https://github\.com/)##; s#\.git$##; s#/$##' <<<"$url" |
			tr '[:upper:]' '[:lower:]')
		if [[ "$slug" != "$want" ]]; then
			release_die "$remote URL $url is not github.com/$repo, where gh opens the PR; run tag-release from a checkout whose $remote is that repository"
		fi
	done <<<"$urls"
}

# Fails unless $tag is greater than every semantic-version v* tag.
release_require_newest() {
	local latest

	latest=$(git tag --list 'v[0-9]*' | release_latest_tag)
	if [[ -n "$latest" ]] && [[ "$("$SEMVER" compare "$tag" "$latest")" != 1 ]]; then
		release_die "$tag must be greater than the latest release tag $latest"
	fi
}

# Cuts $branch from $remote/$base, assembles and checks the changelog, and
# opens the release-preparation PR (sets pr). With nothing to assemble it
# drops the branch and selects the $remote/$base tip instead (sets sha).
release_prepare() {
	local start_branch
	local start_commit
	local refs
	local changes
	local body
	local url

	if git rev-parse -q --verify "refs/heads/$branch" >/dev/null; then
		release_die "local branch $branch exists but has no PR; delete it (git branch -D $branch) and rerun"
	fi
	refs=$(git ls-remote --heads "$remote" "refs/heads/$branch") ||
		release_die "could not query branches on $remote"
	if [[ -n "$refs" ]]; then
		release_die "$remote has branch $branch but no PR; delete it (git push $remote --delete $branch) and rerun"
	fi

	start_branch=$(git symbolic-ref -q --short HEAD || true)
	start_commit=$(git rev-parse HEAD)

	release_log "preparing $branch from $remote/$base"
	git switch -q --no-track -c "$branch" "$remote/$base"
	# shellcheck disable=SC2064 # expand now: the hint names this run's refs
	trap "release_stopped_hint $(printf '%q %q' "$branch" "${start_branch:-$start_commit}")" EXIT

	make --no-print-directory changelog-assemble

	# An empty assembly has no PR diff for changelog-check to accept; it
	# would demand a fragment, so only the release check applies.
	changes=$(git status --porcelain)
	if [[ -z "$changes" ]]; then
		make --no-print-directory changelog-release-check
		release_switch_back "$start_branch" "$start_commit"
		git branch -q -D "$branch"
		trap - EXIT
		sha=$(git rev-parse "$remote/$base")
		release_log "no pending fragments on $remote/$base; tagging its tip $sha without a PR"
		return
	fi

	make --no-print-directory changelog-check CHANGELOG_BASE="$remote/$base"
	make --no-print-directory changelog-release-check

	git add -A -- AICHANGELOG.md .changelog
	git commit -q -m "chore(release): prepare $tag"
	git push -q -u "$remote" "$branch"

	body=$(
		cat <<EOF
Assemble the pending changelog fragments into \`AICHANGELOG.md\` and remove the consumed files before tagging $tag. No application code changes.

Validation:
- \`GOWORK=off make changelog-assemble\`
- \`GOWORK=off make changelog-check CHANGELOG_BASE=origin/main\`
- \`GOWORK=off make changelog-release-check\`

Opened by \`make tag-release\`. Once this PR is merged, $tag is signed on its merge commit and pushed.
EOF
	)
	url=$(gh pr create --repo "$repo" --base "$base" --head "$branch" \
		--title "chore(release): prepare $tag" --body "$body" | tail -n 1)
	pr=${url##*/}
	[[ "$pr" =~ ^[0-9]+$ ]] || release_die "could not read the PR number from gh output: $url"
	release_log "opened $url"

	release_switch_back "$start_branch" "$start_commit"
	trap - EXIT
}

release_switch_back() {
	if [[ -n "$1" ]]; then
		git switch -q "$1"
	else
		git switch -q --detach "$2"
	fi
}

# Polls PR #$pr until it is merged and sets sha to its merge commit. Merging
# stays with GitHub, gated by CI and core approval; this only watches.
release_wait() {
	local query
	local status
	local state
	local oid
	local review
	local checks
	local last=""

	# Tab-separated: state, merge commit, review decision, checks summary.
	# Placeholders keep empty fields from collapsing under IFS=tab.
	# shellcheck disable=SC2016 # $c, $ok and $wait are jq variables
	query='[.statusCheckRollup[]?
		| if .__typename == "StatusContext" then .state
		  elif .status == "COMPLETED" then .conclusion
		  else "PENDING" end] as $c
	| ($c | map(select(. == "SUCCESS" or . == "NEUTRAL" or . == "SKIPPED")) | length) as $ok
	| ($c | map(select(. == "PENDING" or . == "EXPECTED")) | length) as $wait
	| [.state,
	   (.mergeCommit.oid // "-"),
	   (if (.reviewDecision // "") == "" then "none" else .reviewDecision end),
	   "\($ok) passed, \($wait) pending, \(($c | length) - $ok - $wait) failed"]
	| join("\t")'

	release_log "waiting for PR #$pr to be merged; Ctrl-C is safe, rerun with VERSION=$tag to resume"
	while :; do
		if ! status=$(gh pr view "$pr" --repo "$repo" \
			--json state,reviewDecision,mergeCommit,statusCheckRollup --jq "$query"); then
			release_log "could not read PR #$pr; retrying"
			sleep 30
			continue
		fi
		IFS=$'\t' read -r state oid review checks <<<"$status"

		case "$state" in
		MERGED)
			[[ "$oid" != "-" ]] || release_die "PR #$pr is merged but reports no merge commit"
			sha=$oid
			release_log "PR #$pr merged as $sha"
			return
			;;
		CLOSED)
			release_die "PR #$pr was closed without merging; reopen it to resume"
			;;
		esac

		if [[ "$review / $checks" != "$last" ]]; then
			release_log "$(date '+%H:%M:%S') PR #$pr: review $review, checks $checks"
			last="$review / $checks"
		fi
		sleep 30
	done
}

# Signs $tag on $sha and pushes it, after checking what the release workflow
# would reject once the tag is public: $sha must be on $remote/$base, carry
# no pending fragments, and have no other tag.
release_tag() {
	local files
	local pending
	local others
	local refs

	release_log "fetching $remote/$base and tags"
	git fetch -q "$remote" "$base" --tags
	# Another release may have been published while the PR waited.
	release_require_newest

	git merge-base --is-ancestor "$sha" "$remote/$base" ||
		release_die "$sha is not reachable from $remote/$base"

	files=$(git ls-tree -r --name-only "$sha" -- .changelog/)
	pending=$(grep -vxF '.changelog/README.md' <<<"$files" || true)
	if [[ -n "$pending" ]]; then
		release_die "$sha has pending changelog fragments, which the release workflow rejects:
$pending
They merged after $branch was cut; assemble them in another release-preparation PR first (see .changelog/README.md)."
	fi

	others=$(git tag --points-at "$sha" | tr '\n' ' ')
	if [[ -n "${others// /}" ]]; then
		release_die "$sha is already tagged (${others% }); the release workflow requires one tag per commit"
	fi
	refs=$(git ls-remote --tags "$remote" "refs/tags/$tag") ||
		release_die "could not query tags on $remote"
	if [[ -n "$refs" ]]; then
		release_die "$tag already exists on $remote"
	fi

	release_log "signing $tag on $sha"
	git tag -s "$tag" "$sha" -m "Release $tag"
	release_log "pushing $tag to $remote"
	git push -q "$remote" "refs/tags/$tag"
	release_log "pushed $tag; release workflow: https://github.com/$repo/actions/workflows/release.yml"
}

# tag_release runs the maintainer release (SPEC §2.16): prepare and open the
# release-preparation PR, wait for it to merge, then sign and push the tag on
# the merge commit. The tag comes from $VERSION or a single prompt. A rerun
# with the same tag resumes from the release/<tag> PR. The body runs in a
# subshell so strict mode and the cd stay out of the other subcommands.
function tag_release() (
	set -euo pipefail

	local remote=origin
	local base=main
	local repo
	local changes
	local latest
	local tag
	local refs
	local branch
	local found
	local pr=""
	local state=""
	local sha=""

	cd "$SCRIPT_DIR/.."

	is_command git || release_die "git is required"
	is_command gh || release_die "gh is required: https://cli.github.com"
	gh auth status --hostname github.com >/dev/null 2>&1 ||
		release_die "gh is not authenticated here; run gh auth login, or if your gh is a shell function (e.g. the 1Password plugin) pass its token: GH_TOKEN=\$(gh auth token) make tag-release"
	changes=$(git status --porcelain)
	[[ -z "$changes" ]] ||
		release_die "working tree is not clean; commit or stash your changes first"

	repo=$(sed -E 's#^github\.com/##' .github/repo)
	[[ "$repo" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] ||
		release_die ".github/repo does not name a GitHub repository: $repo"
	release_require_canonical_remote

	release_log "fetching $remote/$base and tags"
	git fetch -q "$remote" "$base" --tags
	latest=$(git tag --list 'v[0-9]*' | release_latest_tag)

	tag=${VERSION:-}
	if [[ -z "$tag" ]]; then
		[[ -t 0 ]] || release_die "no terminal to prompt for the tag; pass it as VERSION=vX.Y.Z"
		read -r -p "Release tag (latest ${latest:-none}): " tag || release_die "no tag given"
	fi
	[[ -n "$tag" ]] || release_die "no tag given"
	if [[ "$tag" != v* ]] || [[ "$("$SEMVER" validate "$tag")" != valid ]]; then
		release_die "$tag is not a lowercase-v semantic version such as v1.2.3"
	fi
	refs=$(git ls-remote --tags "$remote" "refs/tags/$tag") ||
		release_die "could not query tags on $remote"
	if [[ -n "$refs" ]]; then
		release_die "$tag already exists on $remote"
	fi
	if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
		release_die "$tag already exists locally; if an earlier run created it without pushing, run git push $remote refs/tags/$tag"
	fi
	release_require_newest

	branch="release/$tag"
	found=$(gh pr list --repo "$repo" --head "$branch" --base "$base" --state all \
		--json number,state --jq 'sort_by(.number) | last | select(. != null) | "\(.number) \(.state)"')
	if [[ -n "$found" ]]; then
		pr=${found% *}
		state=${found#* }
	fi

	case "$state" in
	"")
		release_prepare
		;;
	OPEN | MERGED)
		release_log "resuming $branch: PR #$pr is $state"
		;;
	CLOSED)
		release_die "PR #$pr for $branch was closed without merging; reopen it to resume"
		;;
	*)
		release_die "PR #$pr for $branch is in unexpected state $state"
		;;
	esac

	if [[ -z "$sha" ]]; then
		release_wait
	fi
	release_tag
)

case "$1" in
	gotoolchain)
		get_gotoolchain
		;;
	replace-import-path)
		shift
		replace_import_path "$@"
		;;
	tag-release)
		tag_release
		;;
esac
