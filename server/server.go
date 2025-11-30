package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"limitless/engine"
)

const (
	defaultListenAddr = ":8080"
	defaultSymbol     = "LMT"
)

type server struct {
	book       *engine.OrderBook
	tradeHub   *hub[engine.MatchResult]
	bookHub    *hub[engine.BookView]
	upgrader   websocket.Upgrader
	authToken  string
	corsOrigin string
}

type orderRequest struct {
	ID       string `json:"id"`
	Symbol   string `json:"symbol"`
	Side     string `json:"side"`
	Type     string `json:"type"`
	Price    int64  `json:"price"`
	Quantity int64  `json:"quantity"`
}

type orderResponse struct {
	Status string `json:"status"`
}

type snapshotResponse struct {
	BestBid *publicOrder `json:"bestBid,omitempty"`
	BestAsk *publicOrder `json:"bestAsk,omitempty"`
}

type publicOrder struct {
	ID        string    `json:"id"`
	Symbol    string    `json:"symbol"`
	Side      string    `json:"side"`
	Type      string    `json:"type"`
	Price     int64     `json:"price"`
	Quantity  int64     `json:"quantity"`
	Remaining int64     `json:"remaining"`
	Timestamp time.Time `json:"timestamp"`
}

type outboundMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

func main() {
	listenAddr := getEnv("LISTEN_ADDR", defaultListenAddr)
	symbol := getEnv("SYMBOL", defaultSymbol)
	tickSize := parseIntEnv("TICK_SIZE", 1)
	maxDepth := int(parseIntEnv("MAX_DEPTH", 100))
	authToken := os.Getenv("AUTH_TOKEN")
	corsOrigin := getEnv("CORS_ORIGIN", "*")

	book := engine.NewOrderBook(engine.OrderBookConfig{Symbol: symbol, TickSize: tickSize, MaxDepth: maxDepth})
	srv := newServer(book, authToken, corsOrigin)

	log.Printf("listening on %s for symbol %s", listenAddr, symbol)
	if err := http.ListenAndServe(listenAddr, srv.routes()); err != nil {
		log.Fatal(err)
	}
}

func newServer(book *engine.OrderBook, authToken, corsOrigin string) *server {
	s := &server{
		book:       book,
		tradeHub:   newHub[engine.MatchResult](),
		bookHub:    newHub[engine.BookView](),
		upgrader:   websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
		authToken:  authToken,
		corsOrigin: corsOrigin,
	}

	go s.consumeTrades()
	go s.consumeBookUpdates()
	return s
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/orders", s.withCORS(s.withAuth(http.HandlerFunc(s.handleOrder))))
	mux.Handle("/book", s.withCORS(s.withAuth(http.HandlerFunc(s.handleSnapshot))))
	mux.Handle("/ws/trades", s.withCORS(s.withAuth(http.HandlerFunc(s.handleTradeStream))))
	mux.Handle("/ws/book", s.withCORS(s.withAuth(http.HandlerFunc(s.handleBookStream))))
	return mux
}

func (s *server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", s.corsOrigin)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.authToken == "" {
			next.ServeHTTP(w, r)
			return
		}

		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" {
			token = r.URL.Query().Get("token")
		}
		if token != s.authToken {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("missing or invalid token"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) handleOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req orderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid payload: %w", err))
		return
	}

	order, err := buildOrder(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := s.book.SubmitOrder(order); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	writeJSON(w, http.StatusAccepted, orderResponse{Status: "accepted"})
}

func (s *server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	view, err := s.book.Snapshot()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, snapshotResponse{
		BestBid: toPublicOrder(view.BestBid),
		BestAsk: toPublicOrder(view.BestAsk),
	})
}

func (s *server) handleTradeStream(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	sub := s.tradeHub.Subscribe(32)
	defer s.tradeHub.Unsubscribe(sub)

	for trade := range sub.ch {
		msg := outboundMessage{Type: "trade", Data: toPublicMatch(trade)}
		if err := conn.WriteJSON(msg); err != nil {
			return
		}
	}
}

func (s *server) handleBookStream(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	sub := s.bookHub.Subscribe(32)
	defer s.bookHub.Unsubscribe(sub)

	for view := range sub.ch {
		msg := outboundMessage{Type: "book", Data: snapshotResponse{
			BestBid: toPublicOrder(view.BestBid),
			BestAsk: toPublicOrder(view.BestAsk),
		}}
		if err := conn.WriteJSON(msg); err != nil {
			return
		}
	}
}

func (s *server) consumeTrades() {
	for trade := range s.book.Trades() {
		s.tradeHub.Broadcast(trade)
	}
}

func (s *server) consumeBookUpdates() {
	for view := range s.book.BookUpdates() {
		s.bookHub.Broadcast(view)
	}
}

func buildOrder(req orderRequest) (engine.Order, error) {
	if req.ID == "" || req.Symbol == "" {
		return engine.Order{}, errors.New("id and symbol are required")
	}
	if req.Quantity <= 0 {
		return engine.Order{}, errors.New("quantity must be positive")
	}

	side, err := parseSide(req.Side)
	if err != nil {
		return engine.Order{}, err
	}
	ordType, err := parseOrderType(req.Type)
	if err != nil {
		return engine.Order{}, err
	}

	return engine.Order{
		ID:       req.ID,
		Symbol:   req.Symbol,
		Side:     side,
		Type:     ordType,
		Price:    req.Price,
		Quantity: req.Quantity,
	}, nil
}

func parseSide(value string) (engine.Side, error) {
	switch strings.ToLower(value) {
	case "buy", "bid", "b":
		return engine.Buy, nil
	case "sell", "ask", "s":
		return engine.Sell, nil
	default:
		return 0, fmt.Errorf("unknown side %s", value)
	}
}

func parseOrderType(value string) (engine.OrderType, error) {
	switch strings.ToLower(value) {
	case "limit", "lmt":
		return engine.Limit, nil
	case "market", "mkt":
		return engine.Market, nil
	default:
		return 0, fmt.Errorf("unknown order type %s", value)
	}
}

func toPublicOrder(order *engine.Order) *publicOrder {
	if order == nil {
		return nil
	}
	return &publicOrder{
		ID:        order.ID,
		Symbol:    order.Symbol,
		Side:      sideString(order.Side),
		Type:      typeString(order.Type),
		Price:     order.Price,
		Quantity:  order.Quantity,
		Remaining: order.Remaining,
		Timestamp: order.Timestamp,
	}
}

func toPublicMatch(match engine.MatchResult) map[string]interface{} {
	return map[string]interface{}{
		"symbol":      match.Symbol,
		"buyOrderId":  match.BuyOrderID,
		"sellOrderId": match.SellOrderID,
		"price":       match.Price,
		"quantity":    match.Quantity,
		"executedAt":  match.Timestamp,
	}
}

func sideString(side engine.Side) string {
	if side == engine.Buy {
		return "buy"
	}
	return "sell"
}

func typeString(t engine.OrderType) string {
	if t == engine.Limit {
		return "limit"
	}
	return "market"
}

func writeError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseIntEnv(key string, defaultValue int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		log.Printf("invalid %s value %s: %v, falling back to %d", key, value, err, defaultValue)
		return defaultValue
	}
	return parsed
}
