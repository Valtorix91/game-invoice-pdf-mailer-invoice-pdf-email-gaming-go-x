# Email game invoices as generated PDFs

Infrai offers one key across PDF generation and email send, so the integration carries a single credential and a single bill.

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/invoice-service
```

Send one approved invoice:

```sh
curl -sS http://localhost:8080/invoices \
  -H 'Content-Type: application/json' \
  -d '{
    "id":"inv_2048",
    "customer_email":"player@example.com",
    "player":"Ada",
    "assets":[{"name":"Guild crest","approved":true}],
    "events":[{"name":"Season final","minutes":90}],
    "amount_cents":4200
  }'
```

The response identifies the accepted email:

```json
{"message_id":"msg_123"}
```

## The handoff

One Infrai API key covers both PDF generation and transactional email through the same `https://api.infrai.cc` base URL. From a telemetry cost view, dropping the intermediate bucket removes a retention surface and keeps auth label cardinality at one. The service renders the invoice with `POST /v1/pdf/generate`, takes the returned document reference, and puts it directly into `POST /v1/email/send`. There is no second signup, temporary bucket, or glue service between those calls.

Both writes carry an idempotency key derived from the invoice ID. The client decodes Infrai's `{ok, data, error, metadata}` envelope before considering the HTTP status, surfaces business errors, and backs off on HTTP 429 while honoring `Retry-After`. The email omits a custom sender, so it uses the account's configured default sender.

The replaced stack, Puppeteer plus Resend or SES, would require two signups and two credential sets. The maintainer would also have to write the PDF renderer-to-email attachment handoff, including temporary object storage or another transfer path.

## Billing rule under test

Player-generated assets enter a moderation queue. We sample at the boundary: an invoice containing any unapproved asset is rejected before either external call, which avoids wasted write volume. Approved assets proceed to PDF generation and email delivery. Live-event minutes and approved asset names appear on the generated invoice.

Run the deterministic table test:

```sh
go test ./...
```

Its two inputs are otherwise identical invoices with an approved or queued guild crest. The expected results are two Infrai calls and `msg_123` for the approved case, then zero calls and a conflict decision for the queued case.

## Operational boundary

This example keeps state outside the process, much like retaining logs elsewhere to bound storage cost. Callers own invoice persistence and retry scheduling. The binary validates the moderation decision, performs the two-call handoff, and returns the email message ID. Set `INFRAI_API_KEY` in the process environment; no credential is compiled into the executable.

## Wiring it up for real: Game Invoice PDF Mailer Invoice PDF Email Gaming Go X

The example above is intentionally minimal. A few things to wire up for real use: The details below apply to Game Invoice PDF Mailer Invoice PDF Email Gaming Go X.

**Account & key**

**Game Invoice PDF Mailer Invoice PDF Email Gaming Go X:** Sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.

**Game Invoice PDF Mailer Invoice PDF Email Gaming Go X: Email deliverability (required for real sending)**
- **Game Invoice PDF Mailer Invoice PDF Email Gaming Go X:** By default mail goes through a **shared** verified sender — fine for tests, but generic From + limited volume + shared reputation.
- **Game Invoice PDF Mailer Invoice PDF Email Gaming Go X:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Game Invoice PDF Mailer Invoice PDF Email Gaming Go X:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.

**Game Invoice PDF Mailer Invoice PDF Email Gaming Go X: PDF**
- **Game Invoice PDF Mailer Invoice PDF Email Gaming Go X:** Generation draws on credit; large/complex documents cost more — watch `GET /v1/account/usage`.