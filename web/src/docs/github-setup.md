# GitHub Setup Guide

Connect claude-plane to GitHub to automatically create sessions when repository events occur — pull requests, issues, reviews, and more.

## Prerequisites

- A running claude-plane server with an API key configured.
- A GitHub account with access to the repositories you want to watch.

## Step 1: Create a GitHub Personal Access Token

1. Go to **GitHub Settings > Developer settings > Personal access tokens > Fine-grained tokens**.
2. Click **Generate new token**.
3. Configure the token:
   - **Name:** `claude-plane-bridge`
   - **Expiration:** Choose based on your security policy (90 days recommended).
   - **Repository access:** Select the specific repositories to watch.
   - **Permissions:**
     - **Issues:** Read-only
     - **Pull requests:** Read-only
     - **Checks:** Read-only (if using check run triggers)
     - **Contents:** Read-only (for release triggers)
4. Click **Generate token** and save it securely.

```
github_pat_11AEXAMPLE_AbCdEfGhIjKlMnOpQrStUvWxYz...
```

> Classic tokens also work but fine-grained tokens are recommended for least-privilege access.

## Step 2: Configure the Connector

In the claude-plane web UI:

1. Navigate to **Connectors** in the sidebar.
2. Click **New Connector** and select **GitHub** as the type.
3. Fill in the configuration:

```json
{
  "token": "github_pat_11AEXAMPLE_AbCdEfGhIjKl...",
  "watches": [
    {
      "repo": "your-org/your-repo",
      "template": "code-review",
      "machine_id": "",
      "poll_interval": "60s",
      "triggers": {
        "pull_request_opened": {
          "labels": ["needs-review"],
          "draft": false
        },
        "issue_labeled": {
          "labels": ["claude-review"]
        },
        "issue_comment": {
          "pattern": "@claude-plane"
        }
      }
    }
  ]
}
```

## Configuration Reference

### Top-Level Fields

| Field | Type | Description |
|-------|------|-------------|
| `token` | string | GitHub personal access token |
| `watches` | array | List of repository watch configurations |

### Watch Configuration

| Field | Type | Description |
|-------|------|-------------|
| `repo` | string | Repository in `owner/repo` format |
| `template` | string | Template name for created sessions |
| `machine_id` | string | Target machine (empty = any available) |
| `poll_interval` | string | How often to poll for events (e.g., `"60s"`, `"5m"`) |
| `triggers` | object | Event trigger rules (see below) |

### Trigger Types

Each trigger type can be enabled by including it in the `triggers` object:

| Trigger | Fires When |
|---------|------------|
| `pull_request_opened` | A new PR is opened (filter by labels, draft status) |
| `check_run_completed` | A CI check finishes (filter by conclusion) |
| `issue_labeled` | A label is added to an issue (filter by label names) |
| `issue_comment` | A comment is posted on an issue (filter by pattern) |
| `pull_request_comment` | A comment is posted on a PR (filter by pattern) |
| `pull_request_review` | A PR review is submitted (filter by state) |
| `release_published` | A new release is published |

### Template Variables

When a trigger fires, the connector creates a session using the specified template. These variables are passed to the template:

| Variable | Example | Description |
|----------|---------|-------------|
| `repo` | `your-org/your-repo` | Repository full name |
| `number` | `42` | PR or issue number |
| `title` | `Add feature X` | PR or issue title |
| `author` | `username` | Event author |
| `url` | `https://github.com/...` | Link to the PR/issue |

## Step 3: Set Up Templates

Before the connector can create sessions, you need a matching template:

1. Go to **Templates** in the sidebar.
2. Click **New Template** and create one matching the `template` field in your watch config (e.g., `code-review`).
3. The template prompt can reference variables: `Review PR #{{number}} in {{repo}}: {{title}}`.

## Step 4: Start the Bridge

Configure the bridge to connect to your server:

```toml
# bridge.toml
[claude_plane]
api_url = "https://your-server:4200"
api_key = "your-api-key"

[state]
path = "./bridge-state.json"
```

```bash
./claude-plane-bridge run --config bridge.toml
```

## Step 5: Test the Integration

1. Check the connector status on the **Connectors** page.
2. Open a test PR in your watched repository with the configured label.
3. The connector should detect the event within the poll interval and create a new session.
4. Monitor the **Events** page for `connector.event.matched` events.

## Multiple Repositories

Add multiple entries to the `watches` array to monitor several repositories. Each can have different templates and trigger rules:

```json
{
  "token": "github_pat_...",
  "watches": [
    {
      "repo": "org/frontend",
      "template": "frontend-review",
      "poll_interval": "30s",
      "triggers": {
        "pull_request_opened": { "draft": false }
      }
    },
    {
      "repo": "org/backend",
      "template": "backend-review",
      "poll_interval": "60s",
      "triggers": {
        "pull_request_opened": { "draft": false },
        "issue_labeled": { "labels": ["bug"] }
      }
    }
  ]
}
```

## Troubleshooting

- **No events detected:** Verify the token has read access to the repository. Check that the `poll_interval` has elapsed since the event occurred.
- **Session not created:** Ensure the `template` name matches an existing template in claude-plane. Check the Events page for errors.
- **Rate limiting:** GitHub allows 5,000 requests/hour for authenticated requests. With a 60-second poll interval per repository, you can watch ~80 repositories comfortably.
- **Duplicate sessions:** The connector deduplicates events using unique keys (e.g., `pr:owner/repo:123`). If you see duplicates, check the bridge state file.
