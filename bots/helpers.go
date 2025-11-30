package bots

import "limitless/engine"

func midPrice(view engine.BookView) int64 {
	bid := int64(0)
	ask := int64(0)
	if view.BestBid != nil {
		bid = view.BestBid.Price
	}
	if view.BestAsk != nil {
		ask = view.BestAsk.Price
	}

	switch {
	case bid > 0 && ask > 0:
		return (bid + ask) / 2
	case bid > 0:
		return bid
	case ask > 0:
		return ask
	default:
		return 0
	}
}
