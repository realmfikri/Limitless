# Limitless

A lightweight matching engine with real-time WebSocket streams and a Vite-powered frontend for visualizing trades.

## Backend (Go)
- Start the server: `go run ./server`
- Environment variables:
  - `LISTEN_ADDR` (default `:8080`)
  - `SYMBOL` (default `LMT`)
  - `TICK_SIZE` (default `1`)
  - `MAX_DEPTH` (default `100`)
  - `AUTH_TOKEN` (optional, adds bearer/query auth on all routes)
  - `CORS_ORIGIN` (default `*`)
- Endpoints:
  - `POST /orders` to submit orders
  - `GET /book` for the current best bid/ask
  - `WS /ws/trades` for live fills
  - `WS /ws/book` for book updates

## Frontend (React + Vite)
The UI lives under `web/` and uses TradingView Lightweight Charts to build OHLCV candles from streamed trades, a fast trade tape, and simple play/pause + bot visibility controls.

### Run the frontend
1. Install dependencies: `cd web && npm install`
2. Start the dev server: `npm run dev`
3. Open the printed URL (defaults to `http://localhost:5173`).

The Vite dev server proxies `/orders`, `/book`, and `/ws/*` to `http://localhost:8080`, so you can keep defaults when running the Go backend locally.

### Optional environment variables
- `VITE_WS_URL`: override the websocket base (defaults to the current origin, useful when deploying behind TLS).
- `VITE_AUTH_TOKEN`: bearer token appended as `token=` for websocket auth.

## Combined local flow
1. `go run ./server`
2. `cd web && npm install && npm run dev`
3. Stream trades into the chart and trade tape, pause/resume, and toggle maker/taker-tagged trades from the control panel.
