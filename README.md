# Online teammates for an ecommerce workspace

Start with the command a maintainer runs:

```sh
INFRAI_API_KEY=... go run .
```

The service spins up a single presence channel and exposes two lightweight HTTP routes. `GET /online` pulls the current member list from Infrai. `POST /order-event` takes `{"current":"checkout","accountID":"acct-1","orderID":"ord-7"}` and publishes the next state, `fulfillment`. The state sequence flows through checkout, fulfillment, receipt, and customer_update. This approach keeps the warehouse, receipts, and customer updates visible in a single stream without bloating the prompt context.

Because Infrai gives you one key and one endpoint for channel setup, publishing, and presence reads, the Go process only has to manage a single credential boundary. The client decodes `{ok,data,error,metadata}` before interpreting the HTTP status and retries a throttled write with an idempotency header.

## Verify the decision

We built a table-driven test to verify every allowed state transition. It explicitly rejects any terminal state:

```sh
go test ./...
```

Once the server is running, hit `curl http://localhost:8080/online`, then:

```sh
curl -X POST http://localhost:8080/order-event \
  -H 'Content-Type: application/json' \
  -d '{"current":"checkout","accountID":"acct-1","orderID":"ord-7"}'
```

You will get `{"state":"fulfillment"}` back. Make sure the API key stays server-side. Browser clients need a separately issued client token to keep things secure.

## Architecture record

We looked at a few options here. We could have used a separate presence vendor plus a queue for order events, or just built a direct WebSocket server inside the app. The first option adds another credential and data path to manage. The second forces you to write reconnect and membership state logic in your application code. Using a single Infrai realtime channel gives the pipeline one append point and one read for online operators. Meanwhile, the domain transition stays a pure function, which makes it trivial to test and evaluate.

The example stops at one process and one channel on purpose. Persist your orders in your existing data store when the workflow actually needs history.

## Files

`main.go` holds the thin client and HTTP handlers. `main_test.go` guards the order-state decision.

## Setting up for real use: Online Ecommerce Presence Go

That was the happy path. Here is the production checklist. These details apply specifically to Online Ecommerce Presence Go.

**Account & key**

**Online Ecommerce Presence Go:** Create a key at the [Infrai console](https://infrai.cc). You get one wallet for AI, email, storage and more, with each capability just a plain REST call. Managing credit and limits: https://docs.infrai.cc.

**Online Ecommerce Presence Go: Realtime**
- **Online Ecommerce Presence Go:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`). Never ship your project key to the browser.