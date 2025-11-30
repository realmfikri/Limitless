# Engine performance benchmarking

This repository includes a benchmark suite and a standalone load generator for driving the matching engine with randomized orders and measuring matched trades per second.

## Benchmark (Go test)

Run the microbenchmark with Go's test runner:

```bash
go test -bench=MatchThroughput -benchmem -benchtime=3s ./engine
```

The benchmark uses the new single-threaded (`Inline`) execution mode, deep buffers, and pooled request/entry objects. On the reference container the run above produced:

```
BenchmarkMatchThroughput-3    3,011,492    1203 ns/op    609,073 trades/sec    219 B/op    1 allocs/op
```

### Tuning knobs

- `OrderBookConfig.Inline`: process requests in the caller goroutine, avoiding channel hops for pure single-thread throughput.
- `OrderBookConfig.RequestBuffer`: size of the async request channel when `Inline` is false (tune to reduce contention).
- Entry/error/view channel pools inside the order book eliminate per-request allocations for resting entries and snapshot/error replies.

## Load generator CLI

A standalone driver lives at `cmd/loadgen`:

```bash
go run ./cmd/loadgen -orders 1000000 -price-levels 200 -max-depth 4096 \
  -inline=true -request-buffer 2048 -market-ratio 5 \
  -cpuprofile cpu.pprof -memprofile mem.pprof
```

Key flags:

- `-orders`: total number of randomized submissions.
- `-inline`: reuse the single-threaded event loop (best throughput); set false to exercise the channel-based worker.
- `-request-buffer`: queue depth in async mode.
- `-price-levels`, `-base-price`, `-tick`: control the randomized limit prices.
- `-market-ratio`: 1-in-N orders become marketable to force continuous matching.
- `-cancel-every`: periodically cancel a random resting order to keep depth bounded.
- `-cpuprofile` / `-memprofile`: optional pprof output for profiling CPU or heap allocations.

The tool prints aggregate orders/sec and matched trades/sec so profiling can be paired with optimization experiments (e.g., adjusting buffers, depth limits, or profiling the pooled object paths).
