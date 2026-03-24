#!/usr/bin/env bash
# coverage-comment.sh - Posts test coverage as a PR comment via gh api.
#
# Reads coverage.txt (output of `go tool cover -func=coverage.out`) from the
# current directory, extracts the total coverage percentage, and posts or
# updates a PR comment with the results.
#
# Environment variables:
#   GITHUB_TOKEN      - GitHub token for API access (set automatically in Actions)
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

# Build the comment body.
COVERAGE_DETAILS=$(cat "$COVERAGE_FILE")
COMMENT_BODY="## Test Coverage

**Total: ${TOTAL_COVERAGE}**

<details>
<summary>Per-package breakdown</summary>

\`\`\`
${COVERAGE_DETAILS}
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
    # Update the existing comment.
    gh api "repos/${GITHUB_REPOSITORY}/issues/comments/${EXISTING_COMMENT_ID}" \
      --method PATCH \
      -f body="$COMMENT_BODY" \
      >/dev/null 2>&1 && echo "Updated existing coverage comment." || echo "Failed to update coverage comment (non-fatal)."
  else
    # Create a new comment.
    gh api "repos/${GITHUB_REPOSITORY}/issues/${PR_NUMBER}/comments" \
      --method POST \
      -f body="$COMMENT_BODY" \
      >/dev/null 2>&1 && echo "Posted new coverage comment." || echo "Failed to post coverage comment (non-fatal)."
  fi
}

post_or_update_comment || echo "Coverage comment posting failed (non-fatal)."
exit 0
