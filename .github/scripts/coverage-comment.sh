#!/usr/bin/env bash
# coverage-comment.sh - Posts test coverage as a PR comment via gh api.
#
# Reads coverage.txt (output of `go tool cover -func=coverage.out`) from the
# current directory, extracts the total coverage percentage, and posts or
# updates a PR comment with the results. Filters out generated code and
# zero-coverage packages to stay within GitHub's 65536-char comment limit.
#
# Environment variables:
#   GH_TOKEN          - GitHub token for gh CLI (set by workflow)
#   GITHUB_REPOSITORY - owner/repo (set automatically in Actions)
#   PR_NUMBER         - Pull request number (set by the workflow)

set -euo pipefail

COVERAGE_FILE="coverage.txt"

if [ ! -f "$COVERAGE_FILE" ]; then
  echo "Coverage file not found: $COVERAGE_FILE"
  exit 0
fi

# Extract total coverage percentage from the last line.
# Format: "total:  (statements)  XX.X%"
TOTAL_COVERAGE=$(tail -1 "$COVERAGE_FILE" | awk '{print $NF}')

# Always print to stdout for the CI log.
echo "Total test coverage: $TOTAL_COVERAGE"

# If there is no PR number, we are on a push to main -- skip commenting.
if [ -z "${PR_NUMBER:-}" ]; then
  echo "No PR_NUMBER set, skipping PR comment (likely a push to main)."
  exit 0
fi

# Build a trimmed per-package summary: exclude generated code (ent/) and
# the total line, keep only non-zero coverage, then cap at 100 lines.
COVERAGE_SUMMARY=$(
  grep -v -E '^total:|/ent/|_templ\.go' "$COVERAGE_FILE" \
  | grep -v '0\.0%' \
  | head -100
)
PACKAGE_COUNT=$(echo "$COVERAGE_SUMMARY" | wc -l)

COMMENT_BODY="## Test Coverage

**Total: ${TOTAL_COVERAGE}**

<details>
<summary>Per-function breakdown (${PACKAGE_COUNT} functions with coverage, excludes generated code)</summary>

\`\`\`
${COVERAGE_SUMMARY}
\`\`\`

</details>"

# Post or update the PR comment. All gh api calls are wrapped so failures
# do not break the build -- coverage reporting is informational.
post_or_update_comment() {
  # Search for an existing coverage comment by this bot.
  EXISTING_COMMENT_ID=$(
    gh api "repos/${GITHUB_REPOSITORY}/issues/${PR_NUMBER}/comments" \
      --paginate \
      --jq '.[] | select(.user.login == "github-actions[bot]") | select(.body | contains("## Test Coverage")) | .id' \
      2>/dev/null | head -1
  ) || true

  if [ -n "$EXISTING_COMMENT_ID" ]; then
    if ! echo "$COMMENT_BODY" | gh api "repos/${GITHUB_REPOSITORY}/issues/comments/${EXISTING_COMMENT_ID}" \
      --method PATCH \
      --field body=@- \
      >/dev/null; then
      echo "Failed to update coverage comment (non-fatal)."
    else
      echo "Updated existing coverage comment."
    fi
  else
    if ! echo "$COMMENT_BODY" | gh api "repos/${GITHUB_REPOSITORY}/issues/${PR_NUMBER}/comments" \
      --method POST \
      --field body=@- \
      >/dev/null; then
      echo "Failed to post coverage comment (non-fatal)."
    else
      echo "Posted new coverage comment."
    fi
  fi
}

post_or_update_comment || echo "Coverage comment posting failed (non-fatal)."
exit 0
