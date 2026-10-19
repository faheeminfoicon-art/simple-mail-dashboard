# Simple Mail Dashboard - Render Ready

Consent-based Go email dashboard using go-simple-mail.

## Render configuration
This project includes `Dockerfile` and `render.yaml`.
Set these secret environment variables in Render:
- ADMIN_USER
- ADMIN_PASSWORD
- SMTP_HOST
- SMTP_USERNAME
- SMTP_PASSWORD
- FROM_NAME
- FROM_EMAIL

SMTP encryption defaults to STARTTLS and app port uses Render's PORT variable.

## Storage
The app writes contacts/history to `/var/data/store.json`. For persistence across restarts/redeploys, attach persistent storage at `/var/data` or adapt the app to a managed database.

## Local Docker
`docker build -t simple-mail-dashboard .`
`docker run -p 10000:10000 --env-file .env simple-mail-dashboard`
