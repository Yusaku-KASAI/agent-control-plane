#!/bin/bash
# Tests target/template/bin/update-target: a target copied from version A of the copy source is
# brought to version B. Runs on throwaway repositories made from this checkout's target/.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
w=$(mktemp -d)
trap 'rm -rf "$w"' EXIT
git() { command git -c user.name=test -c user.email=test@example.com -c init.defaultBranch=main "$@"; }
fail() {
	echo "FAIL: $*" >&2
	exit 1
}

# Version A of a stand-in agent-control-plane that holds only the copy source.
mkdir "$w/acp"
cp -R "$root/target" "$w/acp/target"
git -C "$w/acp" init -q
git -C "$w/acp" add -A
git -C "$w/acp" commit -qm A
a=$(git -C "$w/acp" rev-parse HEAD)

# A target copied from A, filled in: a check of its own, and the CODEOWNERS of an organization deleted.
mkdir "$w/t"
git -C "$w/t" init -q
"$w/acp/target/template/bin/copy-target" "$w/t"
[[ $(<"$w/t/template/base") == "$a" ]] || fail "copy-target records the copied commit in template/base"
sed -i 's/^check: check-target$/check: check-target check-own/' "$w/t/Makefile"
echo 'own text' >>"$w/t/README.md"
rm "$w/t/.github/CODEOWNERS"
git -C "$w/t" add -A
git -C "$w/t" commit -qm filled

# Version B: a conflicting change, a clean change, a renamed file, a changed starter, a new file.
sed -i 's/^check: check-target$/check: check-target check-new/' "$w/acp/target/Makefile"
echo '# from B' >>"$w/acp/target/.devcontainer/tmux.conf"
git -C "$w/acp" mv target/.devcontainer/check-vscode-settings.py target/.devcontainer/check-settings.py
echo 'starter text from B' >>"$w/acp/target/README.md.tmpl"
echo 'new in B' >"$w/acp/target/.editorconfig"
git -C "$w/acp" add -A
git -C "$w/acp" commit -qm B
b=$(git -C "$w/acp" rev-parse HEAD)

out=$("$w/acp/target/template/bin/update-target" "$w/t")
echo "$out"

grep -q '^<<<<<<< target' "$w/t/Makefile" || fail "a conflicting change is marked"
grep -qx '  Makefile' <<<"$out" || fail "a conflict is listed"
grep -qx '# from B' "$w/t/.devcontainer/tmux.conf" || fail "a clean change is merged"
[[ ! -e $w/t/.github/CODEOWNERS ]] || fail "a file the target deleted, unchanged in the template, stays deleted"
grep -qx '  .devcontainer/check-vscode-settings.py' <<<"$out" || fail "a renamed file's old name is listed as removed"
[[ -e $w/t/.devcontainer/check-vscode-settings.py ]] || fail "a removed file is left for the target to decide"
grep -q 'own text' "$w/t/README.md" || fail "the target's own text stays"
! grep -q 'starter text from B' "$w/t/README.md" || fail "a starter of the target's own documents is not merged"
[[ -e $w/t/.editorconfig && -e $w/t/.devcontainer/check-settings.py ]] || fail "new files are added"
git -C "$w/t" diff --name-only | grep -qx '.editorconfig' || fail "added files show in git diff"
[[ $(<"$w/t/template/base") == "$b" ]] || fail "template/base moves to the new version"

# A dirty target and a directory outside git are refused.
echo x >"$w/t/dirty"
! "$w/acp/target/template/bin/update-target" "$w/t" 2>/dev/null || fail "a dirty target is refused"
mkdir "$w/plain"
! "$w/acp/target/template/bin/update-target" --base "$a" "$w/plain" 2>/dev/null || fail "a directory outside git is refused"
echo "ok   update-target"
