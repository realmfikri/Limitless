package engine

import (
	"container/heap"
	"errors"
	"fmt"
	"time"
)

type requestType int

const (
	requestAdd requestType = iota
	requestCancel
	requestAmend
	requestSnapshot
	requestStop
)

type bookRequest struct {
	typ        requestType
	order      Order
	amendPrice *int64
	amendQty   *int64
	resp       chan error
	view       chan BookView
}

// OrderBook maintains bids and asks for a single symbol using price-time priority.
type OrderBook struct {
	cfg     OrderBookConfig
	bids    priceTimeQueue
	asks    priceTimeQueue
	orders  map[string]*orderEntry
	seq     int64
	reqCh   chan bookRequest
	trades  chan MatchResult
	updates chan BookView
	now     func() time.Time
}

// NewOrderBook builds an order book and launches the worker loop.
func NewOrderBook(cfg OrderBookConfig) *OrderBook {
	ob := &OrderBook{
		cfg:     cfg,
		bids:    priceTimeQueue{},
		asks:    priceTimeQueue{},
		orders:  make(map[string]*orderEntry),
		reqCh:   make(chan bookRequest),
		trades:  make(chan MatchResult, 16),
		updates: make(chan BookView, 16),
		now:     time.Now,
	}
	heap.Init(&ob.bids)
	heap.Init(&ob.asks)
	go ob.run()
	return ob
}

// SubmitOrder enqueues a new order for processing.
func (ob *OrderBook) SubmitOrder(order Order) error {
	resp := make(chan error, 1)
	ob.reqCh <- bookRequest{typ: requestAdd, order: order, resp: resp}
	return <-resp
}

// CancelOrder cancels an active order by ID.
func (ob *OrderBook) CancelOrder(id string) error {
	resp := make(chan error, 1)
	ob.reqCh <- bookRequest{typ: requestCancel, order: Order{ID: id}, resp: resp}
	return <-resp
}

// AmendOrder updates price and/or quantity for an existing resting order.
func (ob *OrderBook) AmendOrder(id string, price *int64, qty *int64) error {
	resp := make(chan error, 1)
	ob.reqCh <- bookRequest{typ: requestAmend, order: Order{ID: id}, amendPrice: price, amendQty: qty, resp: resp}
	return <-resp
}

// Snapshot returns a view of the best bid and ask for the book.
func (ob *OrderBook) Snapshot() (BookView, error) {
	resp := make(chan error, 1)
	view := make(chan BookView, 1)
	ob.reqCh <- bookRequest{typ: requestSnapshot, resp: resp, view: view}
	return <-view, <-resp
}

// Trades exposes the stream of executed trades.
func (ob *OrderBook) Trades() <-chan MatchResult {
	return ob.trades
}

// BookUpdates exposes the stream of top-of-book updates.
func (ob *OrderBook) BookUpdates() <-chan BookView {
	return ob.updates
}

// Stop gracefully terminates the worker loop.
func (ob *OrderBook) Stop() {
	ob.reqCh <- bookRequest{typ: requestStop}
}

func (ob *OrderBook) run() {
	for req := range ob.reqCh {
		switch req.typ {
		case requestAdd:
			err := ob.processAdd(req.order)
			req.resp <- err
			if err == nil {
				ob.publishView()
			}
		case requestCancel:
			err := ob.processCancel(req.order.ID)
			req.resp <- err
			if err == nil {
				ob.publishView()
			}
		case requestAmend:
			err := ob.processAmend(req.order.ID, req.amendPrice, req.amendQty)
			req.resp <- err
			if err == nil {
				ob.publishView()
			}
		case requestSnapshot:
			ob.handleSnapshot(req.view, req.resp)
		case requestStop:
			close(ob.trades)
			close(ob.updates)
			close(ob.reqCh)
			return
		}
	}
}

func (ob *OrderBook) processAdd(order Order) error {
	if order.Symbol != ob.cfg.Symbol {
		return fmt.Errorf("order symbol %s does not match book %s", order.Symbol, ob.cfg.Symbol)
	}
	if order.Quantity <= 0 {
		return errors.New("order quantity must be positive")
	}
	if order.Type == Limit {
		if ob.cfg.TickSize <= 0 {
			return errors.New("tick size must be positive for limit orders")
		}
		if order.Price <= 0 || order.Price%ob.cfg.TickSize != 0 {
			return fmt.Errorf("price must align to tick size %d", ob.cfg.TickSize)
		}
	}

	ob.seq++
	order.Sequence = ob.seq
	order.Timestamp = ob.now()
	order.Remaining = order.Quantity

	if order.Side == Buy {
		ob.match(&order, &ob.asks, &ob.bids, false)
	} else {
		ob.match(&order, &ob.bids, &ob.asks, true)
	}

	return nil
}

func (ob *OrderBook) match(incoming *Order, opposing *priceTimeQueue, resting *priceTimeQueue, opposingIsBid bool) {
	for incoming.Remaining > 0 {
		best := opposing.peek()
		if best == nil {
			break
		}
		if incoming.Type == Limit {
			if incoming.Side == Buy && incoming.Price < best.order.Price {
				break
			}
			if incoming.Side == Sell && incoming.Price > best.order.Price {
				break
			}
		}

		tradedQty := min(incoming.Remaining, best.order.Remaining)
		tradePrice := best.order.Price
		incoming.Remaining -= tradedQty
		best.order.Remaining -= tradedQty

		ob.trades <- MatchResult{
			Symbol:      incoming.Symbol,
			BuyOrderID:  selectOrderID(incoming, best.order, Buy),
			SellOrderID: selectOrderID(incoming, best.order, Sell),
			Price:       tradePrice,
			Quantity:    tradedQty,
			Timestamp:   ob.now(),
		}

		if best.order.Remaining == 0 {
			heap.Pop(opposing)
			delete(ob.orders, best.order.ID)
		} else {
			heap.Fix(opposing, best.index)
		}
	}

	if incoming.Remaining > 0 && incoming.Type == Limit {
		entry := &orderEntry{order: incoming, isBid: incoming.Side == Buy}
		heap.Push(resting, entry)
		ob.orders[incoming.ID] = entry
		if incoming.Side == Buy {
			trimDepth(resting, ob.cfg.MaxDepth, true, ob.orders)
		} else {
			trimDepth(resting, ob.cfg.MaxDepth, false, ob.orders)
		}
	}
}

func selectOrderID(incoming, resting *Order, side Side) string {
	if incoming.Side == side {
		return incoming.ID
	}
	return resting.ID
}

func (ob *OrderBook) processCancel(id string) error {
	entry, ok := ob.orders[id]
	if !ok {
		return fmt.Errorf("order %s not found", id)
	}
	if entry.isBid {
		ob.bids.remove(entry)
	} else {
		ob.asks.remove(entry)
	}
	delete(ob.orders, id)
	return nil
}

func (ob *OrderBook) processAmend(id string, newPrice *int64, newQty *int64) error {
	entry, ok := ob.orders[id]
	if !ok {
		return fmt.Errorf("order %s not found", id)
	}
	if newQty != nil {
		if *newQty <= 0 {
			return errors.New("amended quantity must be positive")
		}
		entry.order.Quantity = *newQty
		if entry.order.Remaining > *newQty {
			entry.order.Remaining = *newQty
		}
	}
	if newPrice != nil {
		if *newPrice <= 0 || ob.cfg.TickSize <= 0 || *newPrice%ob.cfg.TickSize != 0 {
			return fmt.Errorf("price must align to tick size %d", ob.cfg.TickSize)
		}
		entry.order.Price = *newPrice
	}
	ob.seq++
	entry.order.Sequence = ob.seq
	entry.order.Timestamp = ob.now()

	if entry.isBid {
		heap.Fix(&ob.bids, entry.index)
		trimDepth(&ob.bids, ob.cfg.MaxDepth, true, ob.orders)
	} else {
		heap.Fix(&ob.asks, entry.index)
		trimDepth(&ob.asks, ob.cfg.MaxDepth, false, ob.orders)
	}
	return nil
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func (ob *OrderBook) handleSnapshot(view chan<- BookView, resp chan<- error) {
	view <- ob.snapshotView()
	resp <- nil
}

func (ob *OrderBook) snapshotView() BookView {
	snapshot := BookView{}
	if best := ob.bids.peek(); best != nil {
		copy := *best.order
		snapshot.BestBid = &copy
	}
	if best := ob.asks.peek(); best != nil {
		copy := *best.order
		snapshot.BestAsk = &copy
	}
	return snapshot
}

func (ob *OrderBook) publishView() {
	view := ob.snapshotView()
	select {
	case ob.updates <- view:
	default:
	}
}
