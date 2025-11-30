package bots

import (
	"context"
	"math/rand"
	"time"

	"limitless/engine"
)

// RandomAskBot places short-lived limit asks around the mid price.
type RandomAskBot struct {
	Interval   time.Duration
	Lifetime   time.Duration
	Quantity   int64
	RangeTicks int64
	rand       *rand.Rand
}

func NewRandomAskBot() *RandomAskBot {
	return &RandomAskBot{
		Interval:   200 * time.Millisecond,
		Lifetime:   2 * time.Second,
		Quantity:   1,
		RangeTicks: 5,
		rand:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (b *RandomAskBot) Start(ctx context.Context, client EngineClient) {
	ticker := time.NewTicker(b.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.placeAsk(ctx, client)
		}
	}
}

func (b *RandomAskBot) placeAsk(ctx context.Context, client EngineClient) {
	view, err := client.Snapshot(ctx)
	if err != nil {
		return
	}
	mid := midPrice(view)
	if mid <= 0 {
		return
	}

	delta := b.rand.Int63n(b.RangeTicks+1) * client.TickSize()
	price := mid + delta

	id := client.NextID("ask")
	order := engine.Order{ID: id, Symbol: client.Symbol(), Side: engine.Sell, Type: engine.Limit, Price: price, Quantity: b.Quantity}
	if err := client.SubmitOrder(ctx, order); err != nil {
		return
	}

	go b.cancelAfter(ctx, client, id)
}

func (b *RandomAskBot) cancelAfter(ctx context.Context, client EngineClient, orderID string) {
	timer := time.NewTimer(b.Lifetime)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		_ = client.CancelOrder(context.Background(), orderID)
	}
}
