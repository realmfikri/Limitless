package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"runtime/pprof"
	"strconv"
	"sync/atomic"
	"time"

	"limitless/engine"
)

func main() {
	totalOrders := flag.Int("orders", 500000, "number of orders to submit")
	priceLevels := flag.Int64("price-levels", 200, "unique price levels around the mid")
	tick := flag.Int64("tick", 1, "tick size for limit prices")
	basePrice := flag.Int64("base-price", 10000, "mid price used for randomization")
	symbol := flag.String("symbol", "SIM", "symbol to trade")
	maxDepth := flag.Int("max-depth", 2048, "maximum resting depth")
	cancelEvery := flag.Int("cancel-every", 0, "cancel a random resting order every N submissions")
	inline := flag.Bool("inline", true, "process orders in the caller goroutine instead of over channels")
	reqBuffer := flag.Int("request-buffer", 2048, "queue length for async mode")
	seed := flag.Int64("seed", time.Now().UnixNano(), "seed for deterministic random streams")
	cpuProfile := flag.String("cpuprofile", "", "write cpu profile to file")
	memProfile := flag.String("memprofile", "", "write heap profile to file")
	marketRatio := flag.Int("market-ratio", 5, "1 in N orders will be market instead of limit")
	flag.Parse()

	rng := rand.New(rand.NewSource(*seed))

	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			panic(err)
		}
		defer pprof.StopCPUProfile()
	}

	cfg := engine.OrderBookConfig{Symbol: *symbol, TickSize: *tick, MaxDepth: *maxDepth, RequestBuffer: *reqBuffer, Inline: *inline}
	book := engine.NewOrderBook(cfg)

	var matches int64
	done := make(chan struct{})
	go func() {
		for range book.Trades() {
			atomic.AddInt64(&matches, 1)
		}
		close(done)
	}()

	start := time.Now()
	for i := 0; i < *totalOrders; i++ {
		order := nextRandomOrder(rng, i, *symbol, *basePrice, *priceLevels, *tick, *marketRatio)
		if err := book.SubmitOrder(order); err != nil {
			fmt.Fprintf(os.Stderr, "submit failed: %v\n", err)
		}
		if *cancelEvery > 0 && i > 0 && i%*cancelEvery == 0 {
			target := rng.Intn(i)
			_ = book.CancelOrder("lg-" + strconv.Itoa(target))
		}
	}
	elapsed := time.Since(start)

	book.Stop()
	<-done

	if *memProfile != "" {
		f, err := os.Create(*memProfile)
		if err == nil {
			defer f.Close()
			_ = pprof.WriteHeapProfile(f)
		}
	}

	ordersPerSec := float64(*totalOrders) / elapsed.Seconds()
	tradesPerSec := float64(matches) / elapsed.Seconds()

	fmt.Printf("submitted %d orders in %s (%.0f orders/s)\n", *totalOrders, elapsed.Truncate(time.Millisecond), ordersPerSec)
	fmt.Printf("matched %d trades (%.0f trades/s)\n", matches, tradesPerSec)
	fmt.Printf("config: inline=%t depth=%d request-buffer=%d market-ratio=1/%d\n", *inline, *maxDepth, *reqBuffer, *marketRatio)
}

func nextRandomOrder(rng *rand.Rand, id int, symbol string, mid int64, width int64, tick int64, marketRatio int) engine.Order {
	side := engine.Side(rng.Intn(2))
	var price int64
	if side == engine.Buy {
		price = mid + rng.Int63n(width)
	} else {
		offset := rng.Int63n(width)
		if mid > offset {
			price = mid - offset
		} else {
			price = tick
		}
	}

	otype := engine.Limit
	if marketRatio > 0 && rng.Intn(marketRatio) == 0 {
		otype = engine.Market
	}

	qty := rng.Int63n(5) + 1

	return engine.Order{
		ID:       "lg-" + strconv.Itoa(id),
		Symbol:   symbol,
		Side:     side,
		Type:     otype,
		Price:    price,
		Quantity: qty,
	}
}
