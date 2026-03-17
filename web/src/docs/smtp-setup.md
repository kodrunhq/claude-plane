# SMTP / Email Notification Setup

Configure email notifications to receive alerts when events occur in claude-plane.

## Prerequisites

- A running claude-plane server.
- Access to an SMTP server (your organization's mail server, or a provider like Gmail, SendGrid, or Amazon SES).

## Step 1: Gather SMTP Credentials

You need the following from your email provider:

| Field | Description | Example |
|-------|-------------|---------|
| SMTP Host | Mail server hostname | `smtp.gmail.com` |
| SMTP Port | Server port (usually 587 for TLS, 465 for SSL) | `587` |
| Username | Authentication username (often your email) | `alerts@example.com` |
| Password | Authentication password or app-specific password | `xxxx-xxxx-xxxx-xxxx` |
| From Address | Sender address for outgoing emails | `claude-plane@example.com` |

### Common Provider Settings

| Provider | Host | Port | Notes |
|----------|------|------|-------|
| Gmail | `smtp.gmail.com` | `587` | Requires App Password (not regular password) |
| Outlook/365 | `smtp.office365.com` | `587` | Use your Microsoft account credentials |
| SendGrid | `smtp.sendgrid.net` | `587` | Username is always `apikey`, password is your API key |
| Amazon SES | `email-smtp.{region}.amazonaws.com` | `587` | Use IAM SMTP credentials (not AWS access keys) |
| Mailgun | `smtp.mailgun.org` | `587` | Domain-specific credentials from Mailgun dashboard |

## Step 2: Configure the Notification Channel

Notification channels are configured through the web UI (not TOML files):

1. Navigate to **Settings** in the sidebar.
2. Open the **Notifications** tab.
3. Click **Add Channel** and select **Email**.
4. Fill in the SMTP fields (host, port, username, password, from address, recipient address) and enable TLS if required.
5. Click **Save** to create the channel.

> For Gmail, you must create an **App Password** at https://myaccount.google.com/apppasswords. Regular passwords are rejected when 2FA is enabled.

## Step 3: Subscribe to Events

After configuring SMTP, subscribe to the events you want to receive via email:

1. In the **Settings > Notifications** tab, select the event types to subscribe to.
2. Common choices for email alerts:
   - `session.error` — When a session encounters an error
   - `job.run.failed` — When a job run fails
   - `machine.disconnected` — When an agent machine goes offline
   - `job.run.completed` — When a job run finishes successfully

Select event types and click **Save** to apply your subscription preferences.

## Step 4: Test the Configuration

1. Save your SMTP settings and event subscriptions.
2. Trigger a test event (e.g., create and complete a session).
3. Check your email inbox for the notification.
4. If no email arrives, check:
   - The server logs for SMTP connection errors.
   - Your spam/junk folder.
   - That the event type you triggered is in your subscription list.

## Email Format

Notification emails include:

- **Subject:** Event type and brief summary (e.g., "Session Completed — worker-1")
- **Body:** Event details including timestamp, machine ID, session ID, and relevant metadata
- **From:** The configured sender address

## Gmail App Password Setup

Gmail requires an App Password when 2FA is enabled:

1. Go to https://myaccount.google.com/apppasswords
2. Sign in to your Google account.
3. Select **Mail** as the app and **Other** as the device (enter "claude-plane").
4. Click **Generate** — Google displays a 16-character password.
5. Use this password (not your regular Gmail password) in the SMTP configuration.

## SendGrid Setup

If using SendGrid as your email provider:

1. Sign up at https://sendgrid.com and verify your sender domain.
2. Go to **Settings > API Keys** and create a key with **Mail Send** permission.
3. In the claude-plane web UI, go to **Settings > Notifications > Add Channel > Email** and enter:
   - **Host:** `smtp.sendgrid.net`
   - **Port:** `587`
   - **Username:** `apikey`
   - **Password:** your SendGrid API key (e.g., `SG.your-sendgrid-api-key`)
   - **From:** `notifications@yourdomain.com`
   - **TLS:** enabled

## Troubleshooting

- **Connection refused:** Verify the SMTP host and port. Some networks block port 25; use port 587 (STARTTLS) or 465 (SSL) instead.
- **Authentication failed:** Double-check credentials. For Gmail, ensure you are using an App Password, not your regular password.
- **Emails going to spam:** Set up SPF and DKIM DNS records for your sending domain. Use a consistent "from" address.
- **Timeout errors:** Some corporate firewalls block outbound SMTP. Check with your network administrator or use an HTTP-based provider like SendGrid.
- **TLS errors:** Ensure `tls = true` is set for port 587. For port 465, the connection uses implicit SSL.
