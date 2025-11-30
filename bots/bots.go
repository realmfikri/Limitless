package bots

import (
	"context"

	"limitless/engine"
)

// Bot represents a trading agent that can be run under a supervisor.
type Bot interface {
	Start(ctx context.Context, client EngineClient)
}

// EngineClient abstracts the minimal surface bots need from the matching engine.
type EngineClient interface {
	SubmitOrder(ctx context.Context, order engine.Order) error
	CancelOrder(ctx context.Context, orderID string) error
	Snapshot(ctx context.Context) (engine.BookView, error)
	Trades() <-chan engine.MatchResult
	Symbol() string
	TickSize() int64
	NextID(prefix string) string
	OwnsOrder(id string) bool
}
