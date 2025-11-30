package bots

import (
	"context"
	"time"

	"limitless/engine"
)

// SpreadCaptureBot maintains paired bids/asks and re-prices when the spread moves.
type SpreadCaptureBot struct {
	Interval       time.Duration
	Lifetime       time.Duration
	ThresholdTicks int64
	Quantity       int64
}

type pairedOrders struct {
	buyID     string
	sellID    string
	anchorMid int64
	placedAt  time.Time
}

func NewSpreadCaptureBot() *SpreadCaptureBot {
	return &SpreadCaptureBot{
		Interval:       300 * time.Millisecond,
		Lifetime:       3 * time.Second,
		ThresholdTicks: 3,
		Quantity:       1,
	}
}

func (b *SpreadCaptureBot) Start(ctx context.Context, client EngineClient) {
	ticker := time.NewTicker(b.Interval)
	defer ticker.Stop()

	var pair *pairedOrders
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			view, err := client.Snapshot(ctx)
			if err != nil {
				continue
			}
			pair = b.refreshPair(ctx, client, view, pair)
		}
	}
}

func (b *SpreadCaptureBot) refreshPair(ctx context.Context, client EngineClient, view engine.BookView, pair *pairedOrders) *pairedOrders {
	bid := view.BestBid
	ask := view.BestAsk
	if bid == nil || ask == nil {
		return b.cancelPair(ctx, client, pair)
	}
	mid := (bid.Price + ask.Price) / 2
	threshold := b.ThresholdTicks * client.TickSize()

	if pair != nil {
		if time.Since(pair.placedAt) > b.Lifetime {
			return b.cancelPair(ctx, client, pair)
		}
		if absInt64(mid-pair.anchorMid) >= threshold {
			pair = b.cancelPair(ctx, client, pair)
		}
	}

	if pair != nil {
		return pair
	}

	buyPrice := bid.Price
	if mid-client.TickSize() > 0 {
		buyPrice = mid - client.TickSize()
	}
	sellPrice := ask.Price
	if sellPrice <= buyPrice {
		sellPrice = buyPrice + client.TickSize()
	}

	buyID := client.NextID("spread-bid")
	sellID := client.NextID("spread-ask")

	buyOrder := engine.Order{ID: buyID, Symbol: client.Symbol(), Side: engine.Buy, Type: engine.Limit, Price: buyPrice, Quantity: b.Quantity}
	sellOrder := engine.Order{ID: sellID, Symbol: client.Symbol(), Side: engine.Sell, Type: engine.Limit, Price: sellPrice, Quantity: b.Quantity}

	if err := client.SubmitOrder(ctx, buyOrder); err != nil {
		return pair
	}
	if err := client.SubmitOrder(ctx, sellOrder); err != nil {
		_ = client.CancelOrder(ctx, buyID)
		return pair
	}

	return &pairedOrders{buyID: buyID, sellID: sellID, anchorMid: mid, placedAt: time.Now()}
}

func (b *SpreadCaptureBot) cancelPair(ctx context.Context, client EngineClient, pair *pairedOrders) *pairedOrders {
	if pair == nil {
		return nil
	}
	_ = client.CancelOrder(ctx, pair.buyID)
	_ = client.CancelOrder(ctx, pair.sellID)
	return nil
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
