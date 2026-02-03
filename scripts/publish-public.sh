#!/bin/sh
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
PUBLIC_REPO=${PUBLIC_REPO:-"fulstechlabs/confluence-server-latex-remote-service-public"}
PUBLIC_REPO_URL=${PUBLIC_REPO_URL:-"https://github.com/${PUBLIC_REPO}.git"}
WORK_DIR=$(mktemp -d)

cleanup() {
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

cd "$ROOT_DIR"

if [ -n "$(git status --porcelain)" ]; then
  echo "working tree is dirty; commit or stash before publishing" >&2
  exit 1
fi

git clone "$PUBLIC_REPO_URL" "$WORK_DIR/repo"

# Clear working tree except .git
find "$WORK_DIR/repo" -mindepth 1 -maxdepth 1 ! -name ".git" -exec rm -rf {} +

# Copy current repo contents
rsync -a --delete \
  --exclude ".git" \
  --exclude ".DS_Store" \
  "$ROOT_DIR/" "$WORK_DIR/repo/"

cd "$WORK_DIR/repo"

git add -A
if git diff --cached --quiet; then
  echo "no changes to publish"
  exit 0
fi

GIT_AUTHOR_NAME="John Brown" \
GIT_AUTHOR_EMAIL="support@fulstech.com" \
GIT_COMMITTER_NAME="John Brown" \
GIT_COMMITTER_EMAIL="support@fulstech.com" \
  git commit -m "Sync from private repo"

git push

echo "published to $PUBLIC_REPO"
