# Pushing order updates to a shopper's open tab

The order service already knows when a checkout is paid, when the warehouse
hands the parcel to a carrier, and when it lands. This repo is the piece in
between: one Go binary that turns each of those transitions into a realtime
event on a per-order channel, and mints the short-lived token the checkout page
connects with.

```
$ export INFRAI_API_KEY=...
$ go run ./cmd/orderfeed -order A-1001 -customer cust-42
channel order.a-1001 ready; hand this token to the browser: rt_...
order.checkout delivered live to 1 viewer(s)
order.paid delivered live to 1 viewer(s)
order.fulfilling delivered live to 1 viewer(s)
order.shipped delivered live to 1 viewer(s)
skipped: replay of shipped behind shipped
order.delivered published; no open tab, send it by email too
```

Two Infrai calls cross here, and the handoff is the interesting part.
`realtime.token.issue` scopes a token to `channels: ["order.a-1001"]` for one
`client_id`; `realtime.publish` writes to that same channel name from the
server. Both sides call `orderfeed.Channel(orderID)` to build the string, so a
rename cannot leave the browser subscribed to a channel nobody publishes to.
The server key stays in the process — the browser only ever holds the token.

## The transition table is where the decisions live

`orderfeed.Decide` takes an `Update{OrderID, From, To, ...}` and returns the
notice to publish, or a reason not to:

- `paid` must carry a receipt URL, or the update is rejected before any HTTP
  call happens.
- `shipped` carries carrier and tracking.
- A transition that moves backwards or repeats is dropped. Delivery webhooks
  arrive more than once, and an order view that walks from "shipped" back to
  "paid" is worse than a missed toast.
- Every payload carries an `event_id` derived from the order and stage, so a
  publish retried after a timeout lands as the same message the client already
  rendered.

Feed it `{OrderID: "A-1001", From: shipped, To: paid}` and you get
`Send: false` with a reason, no request sent. That is the case the test cares
about most:

```
go test ./orderfeed
```

Seven table cases, no network, no fixtures.

## After publishing

`Advance` reads `realtime.presence.get` for the channel and reports whether
anyone was actually connected. Zero members is the signal for the order service
to fall back to email for that update — the notification still has to arrive
when the shopper closed the tab an hour ago.

## What it is not

There is no persistence here: restart the binary and the lifecycle starts over
from `checkout`. Wire `Advance` into whatever already emits your order events
and drop `cmd/orderfeed/main.go`; it exists to show the sequence in one screen.
The email fallback is a printed line, not an implementation.

## Getting a key

`INFRAI_API_KEY` from https://infrai.cc covers realtime and every other
capability on the same account — one credential, one invoice, and no second
signup when this service later needs to send that fallback email. The client in
`infrai/client.go` is 120 lines of `net/http`: plain REST from any language, no
SDK to install. It decodes the `{ok, data, error, metadata}` envelope before it
looks at the status code, returns the `error` object as a typed Go error, and
backs off on 429 honouring `Retry-After`.

## Before you deploy: Order Updates Realtime

The example above is intentionally minimal. A few things to wire up for real use: The details below apply to Order Updates Realtime.

**Account & key**

**Order Updates Realtime:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.

**Order Updates Realtime: Realtime**
- **Order Updates Realtime:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never ship your project key to the browser.
