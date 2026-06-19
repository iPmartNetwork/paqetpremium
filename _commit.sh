#!/usr/bin/env bash
cd "$(dirname "$0")" || exit 1
echo '=== bash -n ==='
bash -n install-premium.sh && echo 'BASH_N_PASS' || echo 'BASH_N_FAIL'
echo '=== function defs ==='
grep -nE '^(tunnel_resolve|list_tunnels|cmd_edit|cmd_remove)\(\)' install-premium.sh
echo '=== menu up to 12 ==='
grep -nE '%s12%s\) Uninstall' install-premium.sh
echo '=== git status ==='
git status --porcelain
echo '=== git add ==='
git add -A
echo '=== committed blob CR check ==='
echo "install_CR=$(git show :install-premium.sh | tr -cd '\r' | wc -c)"
echo "changelog_CR=$(git show :CHANGELOG.md | tr -cd '\r' | wc -c)"
echo '=== commit ==='
git commit -m "feat(installer): per-tunnel list/edit/delete management"
echo '=== hash ==='
git rev-parse HEAD
echo '=== push ==='
GIT_TERMINAL_PROMPT=0 git push origin master 2>&1
