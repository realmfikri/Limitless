# Limitless Matching Engine API

This document describes the HTTP and WebSocket interfaces exposed by the local server in `server/`.

## Configuration

Environment variables:

- `LISTEN_ADDR` – address for the HTTP/WebSocket server (default `:8080`).
- `SYMBOL` – trading symbol handled by the book (default `LMT`).
- `TICK_SIZE` – price tick size in integer units (default `1`).
- `MAX_DEPTH` – max resting depth retained in the book (default `100`).
- `CORS_ORIGIN` – value for `Access-Control-Allow-Origin` (default `*`).
- `AUTH_TOKEN` – if set, HTTP and WebSocket calls must include `Authorization: Bearer <token>`.

## HTTP Endpoints

### `POST /orders`
Submit a limit or market order.

**Request body**
```json
{
  "id": "unique-order-id",
  "symbol": "LMT",
  "side": "buy", // or "sell"
  "type": "limit", // or "market"
  "price": 10250,
  "quantity": 10
}
```

**Responses**
- `202 Accepted` on success:
```json
{ "status": "accepted" }
```
- `400 Bad Request` for validation errors.
- `401 Unauthorized` if `AUTH_TOKEN` is configured and missing/invalid.

### `GET /book`
Fetch the current top-of-book snapshot.

**Example response**
```json
{
  "bestBid": {
    "id": "bid-1",
    "symbol": "LMT",
    "side": "buy",
    "type": "limit",
    "price": 10200,
    "quantity": 5,
    "remaining": 5,
    "timestamp": "2024-06-01T12:00:00Z"
  },
  "bestAsk": {
    "id": "ask-1",
    "symbol": "LMT",
    "side": "sell",
    "type": "limit",
    "price": 10300,
    "quantity": 3,
    "remaining": 3,
    "timestamp": "2024-06-01T12:00:05Z"
  }
}
```

## WebSocket Streams

### `GET /ws/trades`
Pushes executions as they occur.

**Message format**
```json
{
  "type": "trade",
  "data": {
    "symbol": "LMT",
    "buyOrderId": "bid-1",
    "sellOrderId": "ask-2",
    "price": 10250,
    "quantity": 2,
    "executedAt": "2024-06-01T12:00:10Z"
  }
}
```

### `GET /ws/book`
Streams book updates (best bid/ask) after every accepted change.

**Message format**
```json
{
  "type": "book",
  "data": {
    "bestBid": { "id": "bid-3", "symbol": "LMT", "side": "buy", "type": "limit", "price": 10200, "quantity": 4, "remaining": 4, "timestamp": "2024-06-01T12:00:12Z" },
    "bestAsk": { "id": "ask-4", "symbol": "LMT", "side": "sell", "type": "limit", "price": 10300, "quantity": 1, "remaining": 1, "timestamp": "2024-06-01T12:00:15Z" }
  }
}
```

## CORS and Authentication
- All HTTP endpoints respond to `OPTIONS` with permissive CORS headers using `CORS_ORIGIN`.
- When `AUTH_TOKEN` is set, clients must send `Authorization: Bearer <token>` on every HTTP request and WebSocket upgrade.

## Notes
- Prices are expressed in integer ticks (`price = dollars / tick_size`).
- Quantity fields are integer units.
- The server only maintains a single symbol per process; deploy multiple instances for multiple symbols.
