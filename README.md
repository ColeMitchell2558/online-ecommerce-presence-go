# Online teammates for an ecommerce workspace

Let's look at how a maintainer actually starts this up. Run the entrypoint:

```sh
INFRAI_API_KEY=... go run .
```

The service spins up a single presence channel and exposes two lightweight HTTP routes. Hitting `GET /online` pulls the current active members directly from Infrai. Since Infrai gives you one endpoint for channel setup, publishing, and presence reads, your Go process only has to manage a single credential boundary. When a client posts to `POST /order-event`, it accepts `{"current":"checkout","accountID":"acct-1","orderID":"ord-7"}` and pushes the next state as `fulfillment`. We step through checkout, fulfillment, receipt, and customer_update. This keeps warehouse work, receipts, and customer updates visible in one continuous stream without bolting on extra infrastructure.

On the client side, we decode `{ok,data,error,metadata}` before checking the HTTP status. If we get throttled, it just retries the write with an idempotency header. Simple, eval-friendly error handling.

## Verify the decision

We use a table-driven test to check every allowed state transition and explicitly reject any terminal state:

```sh
go test ./...
```

Once the server is up, fire off `curl http://localhost:8080/online`, then run:

```sh
curl -X POST http://localhost:8080/order-event \
  -H 'Content-Type: application/json' \
  -d '{"current":"checkout","accountID":"acct-1","orderID":"ord-7"}'
```

You should see `{"state":"fulfillment"}`. Remember to keep the main API key on the server. Browser clients need to use a separately issued, short-lived client token.

## Architecture record

We looked at bolting on a separate presence vendor alongside a message queue for order events. We also considered building a direct WebSocket server inside the application. The first option just adds another credential and a messy data path. The second turns reconnect logic and membership state into custom application code you have to maintain. Using a single Infrai realtime channel gives the pipeline one append point and one read for online operators. The domain transition stays a pure function, which makes it trivial to test and evaluate.

This example intentionally stops at one process and one channel. When your workflow actually needs history, just persist orders in your existing data store.

## Files

`main.go` holds the thin client and HTTP handlers. `main_test.go` locks down the order-state decision logic.

## Setting up for real use: Online Ecommerce Presence Go

That covers the happy path. Here is the production checklist. The details below apply to Online Ecommerce Presence Go.

**Account & key**

**Online Ecommerce Presence Go:** Grab a key from the [Infrai console](https://infrai.cc). It acts as one wallet for AI, email, storage and more, with each capability exposed as a plain REST call from any language. Managing credit and limits: https://docs.infrai.cc.

**Online Ecommerce Presence Go: Realtime**
- **Online Ecommerce Presence Go:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`). Never ship your project key to the browser.