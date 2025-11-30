package engine

import "time"

// Side represents the direction of an order.
type Side int

const (
	// Buy indicates a bid order.
	Buy Side = iota
	// Sell indicates an ask order.
	Sell
)

// OrderType represents the execution style for an order.
type OrderType int

const (
	// Limit orders rest on the book until filled or canceled.
	Limit OrderType = iota
	// Market orders consume available liquidity immediately.
	Market
)

// Order describes a request to trade a symbol.
type Order struct {
	ID        string
	Symbol    string
	Side      Side
	Type      OrderType
	Price     int64 // expressed in ticks
	Quantity  int64
	Remaining int64
	Timestamp time.Time
	Sequence  int64
}

// BookView summarizes top-of-book information for a symbol.
type BookView struct {
	BestBid *Order
	BestAsk *Order
}

// MatchResult captures a completed trade.
type MatchResult struct {
	Symbol      string
	BuyOrderID  string
	SellOrderID string
	Price       int64
	Quantity    int64
	Timestamp   time.Time
}

// OrderBookConfig controls book parameters.
type OrderBookConfig struct {
	Symbol   string
	TickSize int64
	MaxDepth int
}
