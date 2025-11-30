package engine

import (
	"fmt"
	"math/rand"
	"strconv"
	"sync/atomic"
	"testing"
)

func BenchmarkMatchThroughput(b *testing.B) {
	cfg := OrderBookConfig{Symbol: "SIM", TickSize: 1, MaxDepth: 2048, RequestBuffer: 2048, Inline: true}
	ob := NewOrderBook(cfg)
	defer ob.Stop()

	randGen := rand.New(rand.NewSource(42))

	var matched int64
	done := make(chan struct{})
	go func() {
		for range ob.Trades() {
			atomic.AddInt64(&matched, 1)
		}
		close(done)
	}()

	orders := make([]Order, b.N)
	for i := 0; i < b.N; i++ {
		orders[i] = randomBenchmarkOrder(randGen, i)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := ob.SubmitOrder(orders[i]); err != nil {
			b.Fatalf("submit failed: %v", err)
		}
	}

	ob.Stop()
	<-done
	b.StopTimer()

	if elapsed := b.Elapsed(); elapsed > 0 {
		tradesPerSecond := float64(matched) / elapsed.Seconds()
		b.ReportMetric(tradesPerSecond, "trades/sec")
	}
}

func randomBenchmarkOrder(rng *rand.Rand, idx int) Order {
	side := Side(rng.Intn(2))
	var price int64
	base := int64(10_000)
	width := int64(100)
	if side == Buy {
		price = base + rng.Int63n(width)
	} else {
		price = base - rng.Int63n(width)
		if price <= 0 {
			price = 1
		}
	}

	otype := Limit
	if rng.Intn(5) == 0 {
		otype = Market
	}

	return Order{
		ID:       fmt.Sprintf("bench-%s", strconv.Itoa(idx)),
		Symbol:   "SIM",
		Side:     side,
		Type:     otype,
		Price:    price,
		Quantity: rng.Int63n(5) + 1,
	}
}
