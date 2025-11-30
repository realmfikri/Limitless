package bots

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"limitless/engine"
)

// Supervisor orchestrates multiple bots with a shared client and PnL tracking.
type Supervisor struct {
	bots     []Bot
	client   *ThrottledClient
	pnl      *pnlTracker
	throttle *time.Ticker
}

// NewSupervisor builds a default swarm of bots and a throttled client.
func NewSupervisor(book *engine.OrderBook, cfg engine.OrderBookConfig, orderInterval time.Duration) *Supervisor {
	throttle := time.NewTicker(orderInterval)
	client := NewThrottledClient(book, cfg.Symbol, cfg.TickSize, throttle.C)
	bots := []Bot{
		NewRandomBidBot(),
		NewRandomAskBot(),
		NewRandomBidBot(),
		NewRandomAskBot(),
		NewSpreadCaptureBot(),
	}
	return &Supervisor{
		bots:     bots,
		client:   client,
		pnl:      &pnlTracker{},
		throttle: throttle,
	}
}

// Start launches all bots and PnL monitoring until the context is canceled.
func (s *Supervisor) Start(ctx context.Context) {
	logTicker := time.NewTicker(2 * time.Second)
	defer logTicker.Stop()
	defer s.throttle.Stop()

	for _, bot := range s.bots {
		b := bot
		go b.Start(ctx, s.client)
	}

	go s.consumeTrades(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-logTicker.C:
			pos, cash := s.pnl.Snapshot()
			log.Printf("PNL position=%d cash=%d", pos, cash)
		}
	}
}

func (s *Supervisor) consumeTrades(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case trade, ok := <-s.client.Trades():
			if !ok {
				return
			}
			s.pnl.Record(trade, s.client)
		}
	}
}

type pnlTracker struct {
	mu       sync.Mutex
	position int64
	cash     int64
}

func (p *pnlTracker) Record(trade engine.MatchResult, client EngineClient) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if client.OwnsOrder(trade.BuyOrderID) {
		p.position += trade.Quantity
		p.cash -= trade.Price * trade.Quantity
	}
	if client.OwnsOrder(trade.SellOrderID) {
		p.position -= trade.Quantity
		p.cash += trade.Price * trade.Quantity
	}
}

func (p *pnlTracker) Snapshot() (int64, int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.position, p.cash
}

// RunExampleSupervisor demonstrates spinning up the supervisor with a fresh book.
func RunExampleSupervisor() {
	cfg := engine.OrderBookConfig{Symbol: "SIM", TickSize: 1, MaxDepth: 50}
	book := engine.NewOrderBook(cfg)
	sup := NewSupervisor(book, cfg, 50*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sup.Start(ctx)
	book.Stop()
	fmt.Printf("final PNL position=%d cash=%d\n", sup.pnl.position, sup.pnl.cash)
}
