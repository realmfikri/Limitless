package engine

import "container/heap"

// orderEntry wraps an order for heap operations.
type orderEntry struct {
	order *Order
	index int
	isBid bool
}

// priceTimeQueue implements a price-time priority queue.
type priceTimeQueue []*orderEntry

func (q priceTimeQueue) Len() int { return len(q) }

func (q priceTimeQueue) Less(i, j int) bool {
	// For bids: higher price has priority, then older timestamp/sequence.
	// For asks: lower price has priority, then older timestamp/sequence.
	a, b := q[i], q[j]
	if a.order.Price != b.order.Price {
		if a.isBid {
			return a.order.Price > b.order.Price
		}
		return a.order.Price < b.order.Price
	}
	if !a.order.Timestamp.Equal(b.order.Timestamp) {
		return a.order.Timestamp.Before(b.order.Timestamp)
	}
	return a.order.Sequence < b.order.Sequence
}

func (q priceTimeQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index = i
	q[j].index = j
}

func (q *priceTimeQueue) Push(x any) {
	entry := x.(*orderEntry)
	entry.index = len(*q)
	*q = append(*q, entry)
}

func (q *priceTimeQueue) Pop() any {
	old := *q
	n := len(old)
	entry := old[n-1]
	entry.index = -1
	*q = old[0 : n-1]
	return entry
}

func (q priceTimeQueue) peek() *orderEntry {
	if len(q) == 0 {
		return nil
	}
	return q[0]
}

func (q *priceTimeQueue) remove(entry *orderEntry) *orderEntry {
	return heap.Remove(q, entry.index).(*orderEntry)
}

func (q *priceTimeQueue) findWorstIndex(isBid bool) int {
	if len(*q) == 0 {
		return -1
	}
	worstIdx := 0
	for i := range *q {
		if isBid {
			if (*q)[i].order.Price < (*q)[worstIdx].order.Price {
				worstIdx = i
			} else if (*q)[i].order.Price == (*q)[worstIdx].order.Price {
				if (*q)[i].order.Timestamp.After((*q)[worstIdx].order.Timestamp) {
					worstIdx = i
				}
			}
		} else {
			if (*q)[i].order.Price > (*q)[worstIdx].order.Price {
				worstIdx = i
			} else if (*q)[i].order.Price == (*q)[worstIdx].order.Price {
				if (*q)[i].order.Timestamp.After((*q)[worstIdx].order.Timestamp) {
					worstIdx = i
				}
			}
		}
	}
	return worstIdx
}

func trimDepth(q *priceTimeQueue, maxDepth int, isBid bool, orderIndex map[string]*orderEntry, release func(*orderEntry)) {
	for maxDepth > 0 && q.Len() > maxDepth {
		idx := q.findWorstIndex(isBid)
		if idx < 0 {
			return
		}
		entry := heap.Remove(q, idx).(*orderEntry)
		delete(orderIndex, entry.order.ID)
		if release != nil {
			release(entry)
		}
	}
}
