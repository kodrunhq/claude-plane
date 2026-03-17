# Telegram Setup Guide

Connect claude-plane to Telegram to receive event notifications in a group chat and trigger jobs via bot commands.

## Prerequisites

- A running claude-plane server with an API key configured.
- A Telegram account to create the bot.
- A Telegram group (supergroup recommended) where the bot will post.

## Step 1: Create a Telegram Bot

1. Open Telegram and search for **@BotFather**.
2. Send `/newbot` and follow the prompts to choose a name and username.
3. BotFather responds with your **bot token** — save this securely. It looks like:

```
7123456789:AAH1bGciOiJIUzI1NiIsInR5cCI6Ikp
```

> Keep your bot token secret. Anyone with the token can control your bot.

## Step 2: Create a Group and Add the Bot

1. Create a new Telegram group (or use an existing one).
2. Add your bot to the group as a member.
3. **Recommended:** Convert the group to a **supergroup** (Settings > Group Type) to enable topic threads.
4. If using topics, create two topics:
   - **Events** — for receiving event notifications from claude-plane.
   - **Commands** — for sending commands to claude-plane.

## Step 3: Get the Group ID and Topic IDs

The easiest way to find your group and topic IDs:

1. Send a message in the group.
2. Open `https://api.telegram.org/bot<YOUR_BOT_TOKEN>/getUpdates` in a browser.
3. Look for the `chat.id` field — this is your **group ID** (typically a negative number like `-1001234567890`).
4. If using topics, the `message_thread_id` field in each message shows the **topic ID**.

Alternatively, forward a message from the group to `@userinfobot` to get the chat ID.

## Step 4: Configure the Connector

In the claude-plane web UI:

1. Navigate to **Connectors** in the sidebar.
2. Click **New Connector** and select **Telegram** as the type.
3. Fill in the configuration:

```json
{
  "bot_token": "7123456789:AAH1bGciOiJIUzI1NiIsInR5cCI6Ikp",
  "group_id": -1001234567890,
  "events_topic_id": 1,
  "commands_topic_id": 2,
  "poll_timeout": 30,
  "event_types": [
    "session.created",
    "session.completed",
    "job.run.started",
    "job.run.completed",
    "machine.connected",
    "machine.disconnected"
  ]
}
```

| Field | Description |
|-------|-------------|
| `bot_token` | The token from BotFather |
| `group_id` | Your Telegram group's numeric ID |
| `events_topic_id` | Topic ID for posting event notifications (0 for no topics) |
| `commands_topic_id` | Topic ID for receiving commands (0 for no topics) |
| `poll_timeout` | Long-polling timeout in seconds (default: 30) |
| `event_types` | List of claude-plane event types to forward |

## Step 5: Start the Bridge

If running the bridge as a standalone binary:

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

The bridge polls the Telegram API for incoming commands and forwards subscribed events as formatted messages to the configured group/topic.

## Step 6: Test the Connection

1. Check the connector status on the **Connectors** page — it should show as healthy.
2. Create a test session in claude-plane. If `session.created` is in your event types, a notification should appear in the Telegram events topic.
3. Try sending `/status` in the commands topic to verify the bot responds.

## Available Bot Commands

| Command | Description |
|---------|-------------|
| `/status` | Show bridge and server health |
| `/sessions` | List active sessions |
| `/machines` | List connected machines |

## Troubleshooting

- **Bot not responding:** Verify the bot token is correct and the bot is a member of the group.
- **No notifications:** Check that the `event_types` list includes the events you expect. Verify the `events_topic_id` is correct.
- **Rate limiting:** Telegram limits bots to ~30 messages per second. The connector handles rate limits automatically with retry-after backoff.
- **Group ID wrong sign:** Group IDs for supergroups start with `-100`. Regular groups start with `-`. Ensure you have the full number.
