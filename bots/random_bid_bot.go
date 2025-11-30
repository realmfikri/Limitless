package bots

import (
	"context"
	"math/rand"
	"time"

	"limitless/engine"
)

// RandomBidBot places short-lived limit bids around the mid price.
type RandomBidBot struct {
	Interval   time.Duration
	Lifetime   time.Duration
	Quantity   int64
	RangeTicks int64
	rand       *rand.Rand
}

func NewRandomBidBot() *RandomBidBot {
	return &RandomBidBot{
		Interval:   200 * time.Millisecond,
		Lifetime:   2 * time.Second,
		Quantity:   1,
		RangeTicks: 5,
		rand:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (b *RandomBidBot) Start(ctx context.Context, client EngineClient) {
	ticker := time.NewTicker(b.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.placeBid(ctx, client)
		}
	}
}

func (b *RandomBidBot) placeBid(ctx context.Context, client EngineClient) {
	view, err := client.Snapshot(ctx)
	if err != nil {
		return
	}
	mid := midPrice(view)
	if mid <= 0 {
		return
	}

	delta := b.rand.Int63n(b.RangeTicks+1) * client.TickSize()
	price := mid - delta
	if price <= 0 {
		price = client.TickSize()
	}

	id := client.NextID("bid")
	order := engine.Order{ID: id, Symbol: client.Symbol(), Side: engine.Buy, Type: engine.Limit, Price: price, Quantity: b.Quantity}
	if err := client.SubmitOrder(ctx, order); err != nil {
		return
	}

	go b.cancelAfter(ctx, client, id)
}

func (b *RandomBidBot) cancelAfter(ctx context.Context, client EngineClient, orderID string) {
	timer := time.NewTimer(b.Lifetime)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		_ = client.CancelOrder(context.Background(), orderID)
	}
}
