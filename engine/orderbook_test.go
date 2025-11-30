package engine

import (
	"testing"
	"time"
)

func TestLimitMatch(t *testing.T) {
	ob := NewOrderBook(OrderBookConfig{Symbol: "BTCUSD", TickSize: 1, MaxDepth: 10})
	defer ob.Stop()

	ob.now = func() time.Time { return time.Unix(0, 0) }

	if err := ob.SubmitOrder(Order{ID: "ask1", Symbol: "BTCUSD", Side: Sell, Type: Limit, Price: 101, Quantity: 5}); err != nil {
		t.Fatalf("failed to add ask: %v", err)
	}

	ob.now = func() time.Time { return time.Unix(1, 0) }
	if err := ob.SubmitOrder(Order{ID: "bid1", Symbol: "BTCUSD", Side: Buy, Type: Limit, Price: 102, Quantity: 3}); err != nil {
		t.Fatalf("failed to add bid: %v", err)
	}

	trade := <-ob.Trades()
	if trade.Quantity != 3 || trade.Price != 101 {
		t.Fatalf("unexpected trade: %+v", trade)
	}
}

func TestMarketOrderConsumesBest(t *testing.T) {
	ob := NewOrderBook(OrderBookConfig{Symbol: "ETHUSD", TickSize: 1, MaxDepth: 10})
	defer ob.Stop()
	ob.now = func() time.Time { return time.Unix(0, 0) }

	_ = ob.SubmitOrder(Order{ID: "ask1", Symbol: "ETHUSD", Side: Sell, Type: Limit, Price: 50, Quantity: 2})
	_ = ob.SubmitOrder(Order{ID: "ask2", Symbol: "ETHUSD", Side: Sell, Type: Limit, Price: 55, Quantity: 5})

	ob.now = func() time.Time { return time.Unix(1, 0) }
	if err := ob.SubmitOrder(Order{ID: "mkt1", Symbol: "ETHUSD", Side: Buy, Type: Market, Quantity: 4}); err != nil {
		t.Fatalf("submit market order: %v", err)
	}

	trade1 := <-ob.Trades()
	trade2 := <-ob.Trades()

	if trade1.Price != 50 || trade1.Quantity != 2 {
		t.Fatalf("unexpected first trade %+v", trade1)
	}
	if trade2.Price != 55 || trade2.Quantity != 2 {
		t.Fatalf("unexpected second trade %+v", trade2)
	}
}

func TestAmendAndCancel(t *testing.T) {
	ob := NewOrderBook(OrderBookConfig{Symbol: "SOLUSD", TickSize: 1, MaxDepth: 5})
	defer ob.Stop()
	ob.now = func() time.Time { return time.Unix(0, 0) }

	if err := ob.SubmitOrder(Order{ID: "bid1", Symbol: "SOLUSD", Side: Buy, Type: Limit, Price: 10, Quantity: 1}); err != nil {
		t.Fatalf("failed to add bid1: %v", err)
	}
	if err := ob.SubmitOrder(Order{ID: "bid2", Symbol: "SOLUSD", Side: Buy, Type: Limit, Price: 9, Quantity: 1}); err != nil {
		t.Fatalf("failed to add bid2: %v", err)
	}

	newPrice := int64(8)
	if err := ob.AmendOrder("bid2", &newPrice, nil); err != nil {
		t.Fatalf("amend failed: %v", err)
	}

	if err := ob.CancelOrder("bid1"); err != nil {
		t.Fatalf("cancel failed: %v", err)
	}

	if err := ob.SubmitOrder(Order{ID: "ask1", Symbol: "SOLUSD", Side: Sell, Type: Limit, Price: 8, Quantity: 1}); err != nil {
		t.Fatalf("failed to add ask: %v", err)
	}

	trade := <-ob.Trades()
	if trade.BuyOrderID != "bid2" || trade.Price != 8 {
		t.Fatalf("unexpected trade %+v", trade)
	}
}

func TestMaxDepthTrimming(t *testing.T) {
	ob := NewOrderBook(OrderBookConfig{Symbol: "ADAUSD", TickSize: 1, MaxDepth: 2})
	defer ob.Stop()
	ob.now = func() time.Time { return time.Unix(0, 0) }

	_ = ob.SubmitOrder(Order{ID: "bid1", Symbol: "ADAUSD", Side: Buy, Type: Limit, Price: 10, Quantity: 1})
	ob.now = func() time.Time { return time.Unix(1, 0) }
	_ = ob.SubmitOrder(Order{ID: "bid2", Symbol: "ADAUSD", Side: Buy, Type: Limit, Price: 9, Quantity: 1})
	ob.now = func() time.Time { return time.Unix(2, 0) }
	_ = ob.SubmitOrder(Order{ID: "bid3", Symbol: "ADAUSD", Side: Buy, Type: Limit, Price: 8, Quantity: 1})

	if len(ob.bids) != 2 {
		t.Fatalf("expected bids trimmed to depth 2, got %d", len(ob.bids))
	}
	if _, ok := ob.orders["bid3"]; ok {
		t.Fatalf("lowest priority order should have been trimmed")
	}
}

func TestSnapshotCopiesTopLevels(t *testing.T) {
	ob := NewOrderBook(OrderBookConfig{Symbol: "XRPUSD", TickSize: 1, MaxDepth: 5})
	defer ob.Stop()
	ob.now = func() time.Time { return time.Unix(0, 0) }

	_ = ob.SubmitOrder(Order{ID: "bid1", Symbol: "XRPUSD", Side: Buy, Type: Limit, Price: 10, Quantity: 1})
	_ = ob.SubmitOrder(Order{ID: "ask1", Symbol: "XRPUSD", Side: Sell, Type: Limit, Price: 12, Quantity: 1})

	view, err := ob.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if view.BestBid == nil || view.BestAsk == nil {
		t.Fatalf("expected best levels present in snapshot: %+v", view)
	}

	view.BestBid.Price = 1
	second, _ := ob.Snapshot()
	if second.BestBid.Price != 10 {
		t.Fatalf("snapshot should return copies, expected 10 got %d", second.BestBid.Price)
	}
}
