package bots

import (
	"context"
	"fmt"
	"sync"
	"time"

	"limitless/engine"
)

type ThrottledClient struct {
	book     *engine.OrderBook
	symbol   string
	tickSize int64
	throttle <-chan time.Time
	trades   <-chan engine.MatchResult
	mu       sync.Mutex
	orderSeq int64
	owned    map[string]struct{}
}

// NewThrottledClient wraps an order book with basic rate limiting and bookkeeping.
func NewThrottledClient(book *engine.OrderBook, symbol string, tickSize int64, throttle <-chan time.Time) *ThrottledClient {
	return &ThrottledClient{
		book:     book,
		symbol:   symbol,
		tickSize: tickSize,
		throttle: throttle,
		trades:   book.Trades(),
		owned:    make(map[string]struct{}),
	}
}

func (c *ThrottledClient) waitThrottle(ctx context.Context) error {
	if c.throttle == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.throttle:
		return nil
	}
}

func (c *ThrottledClient) SubmitOrder(ctx context.Context, order engine.Order) error {
	if err := c.waitThrottle(ctx); err != nil {
		return err
	}
	if order.Symbol == "" {
		order.Symbol = c.symbol
	}
	if order.Price > 0 && order.Price%c.tickSize != 0 {
		order.Price = (order.Price / c.tickSize) * c.tickSize
	}
	if err := c.book.SubmitOrder(order); err != nil {
		return err
	}
	c.mu.Lock()
	c.owned[order.ID] = struct{}{}
	c.mu.Unlock()
	return nil
}

func (c *ThrottledClient) CancelOrder(ctx context.Context, orderID string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return c.book.CancelOrder(orderID)
}

func (c *ThrottledClient) Snapshot(ctx context.Context) (engine.BookView, error) {
	type result struct {
		view engine.BookView
		err  error
	}
	done := make(chan result, 1)
	go func() {
		view, err := c.book.Snapshot()
		done <- result{view: view, err: err}
	}()

	select {
	case <-ctx.Done():
		return engine.BookView{}, ctx.Err()
	case res := <-done:
		return res.view, res.err
	}
}

func (c *ThrottledClient) Trades() <-chan engine.MatchResult {
	return c.trades
}

func (c *ThrottledClient) Symbol() string {
	return c.symbol
}

func (c *ThrottledClient) TickSize() int64 {
	return c.tickSize
}

func (c *ThrottledClient) NextID(prefix string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.orderSeq++
	return fmt.Sprintf("%s-%d", prefix, c.orderSeq)
}

func (c *ThrottledClient) OwnsOrder(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.owned[id]
	return ok
}
