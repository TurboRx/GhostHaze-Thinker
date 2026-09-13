# Security Policy

## Supported Versions

We provide security updates and patches for the following versions:

| Version | Supported          |
| ------- | ------------------ |
| `main`  | :white_check_mark: |
| < 1.0   | :x:                |

We strongly encourage all users to run the latest commit from the `main` branch or the latest container release.

## Reporting a Vulnerability

We take the security of GhostHaze-Thinker seriously. If you identify a security vulnerability, please do **NOT** open a public issue.

Instead, please report security issues through:
1. **GitHub Private Vulnerability Reporting**: Use the "Report a vulnerability" button under the **Security** tab of the repository.
2. If private reporting is unavailable, contact the repository maintainers directly with details.

### What to Include

When reporting a potential vulnerability, please provide:
- A description of the issue and its potential impact.
- Clear steps to reproduce the issue or a proof-of-concept.
- Any suggested remediations or patches if available.

### Response Timeline

- **Initial acknowledgment**: Within 48 hours of receipt.
- **Triage & validation**: Within 7 days.
- **Resolution & disclosure**: We will coordinate with you to deploy a fix and agree upon a public disclosure date once patches are verified.

## Security Best Practices for Deployments

- Always set a strong, random password for `WEB_ADMIN_PASSWORD` in production.
- Keep bot account credentials stored securely in environment variables or `.env` rather than hardcoded in source code.
- Restrict web panel exposure to trusted networks or place behind a secure reverse proxy (e.g. Cloudflare Access, Traefik, or Caddy) with TLS enabled.
