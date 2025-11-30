import { useEffect, useMemo, useRef, useState } from 'react'
import {
  CandlestickData,
  CandlestickSeries,
  ColorType,
  HistogramData,
  HistogramSeries,
  ISeriesApi,
  UTCTimestamp,
  createChart,
} from 'lightweight-charts'
import './App.css'

const CANDLE_INTERVAL_SECONDS = 60

const getWsUrl = (path: string, token?: string) => {
  const base = import.meta.env.VITE_WS_URL as string | undefined
  const prefix = base
    ? base.replace(/\/$/, '')
    : `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}`
  const separator = path.includes('?') ? '&' : '?'
  const query = token ? `${separator}token=${encodeURIComponent(token)}` : ''
  return `${prefix}${path}${query}`
}

type Trade = {
  symbol: string
  price: number
  quantity: number
  buyOrderId: string
  sellOrderId: string
  executedAt: string
}

type Candle = CandlestickData & { time: UTCTimestamp; volume: number }

const normalizeTrade = (payload: any): Trade => ({
  symbol: payload?.symbol ?? 'LMT',
  price: Number(payload?.price ?? 0),
  quantity: Number(payload?.quantity ?? 0),
  buyOrderId: payload?.buyOrderId ?? payload?.buyOrderID ?? '',
  sellOrderId: payload?.sellOrderId ?? payload?.sellOrderID ?? '',
  executedAt: payload?.executedAt ?? payload?.timestamp ?? new Date().toISOString(),
})

function App() {
  const [trades, setTrades] = useState<Trade[]>([])
  const [isPlaying, setIsPlaying] = useState(true)
  const [connectionStatus, setConnectionStatus] = useState<'connecting' | 'live' | 'paused' | 'error'>(
    'connecting',
  )
  const [botVisibility, setBotVisibility] = useState({ maker: true, taker: true })
  const [reconnectToken, setReconnectToken] = useState(0)

  const chartContainerRef = useRef<HTMLDivElement | null>(null)
  const tradeTapeRef = useRef<HTMLDivElement | null>(null)
  const candleSeriesRef = useRef<ISeriesApi<'Candlestick'> | null>(null)
  const volumeSeriesRef = useRef<ISeriesApi<'Histogram'> | null>(null)

  const filteredTrades = useMemo(() => {
    return trades.filter((trade) => {
      const lower = `${trade.buyOrderId} ${trade.sellOrderId}`.toLowerCase()
      const containsMaker = lower.includes('maker')
      const containsTaker = lower.includes('taker')

      if (containsMaker && !botVisibility.maker) return false
      if (containsTaker && !botVisibility.taker) return false
      return true
    })
  }, [trades, botVisibility])

  const candles = useMemo<Candle[]>(() => {
    const buckets = new Map<UTCTimestamp, Candle>()

    filteredTrades.forEach((trade) => {
      const timestampMs = new Date(trade.executedAt).getTime()
      const bucketStart =
        (Math.floor(timestampMs / (CANDLE_INTERVAL_SECONDS * 1000)) * CANDLE_INTERVAL_SECONDS) as UTCTimestamp
      const existing = buckets.get(bucketStart)
      const price = trade.price

      if (!existing) {
        buckets.set(bucketStart, {
          time: bucketStart,
          open: price,
          high: price,
          low: price,
          close: price,
          volume: trade.quantity,
        })
      } else {
        existing.high = Math.max(existing.high, price)
        existing.low = Math.min(existing.low, price)
        existing.close = price
        existing.volume += trade.quantity
      }
    })

    return Array.from(buckets.values()).sort((a, b) => Number(a.time) - Number(b.time))
  }, [filteredTrades])

  const volumeBars = useMemo<HistogramData[]>(
    () =>
      candles.map((candle) => ({
        time: candle.time,
        value: candle.volume,
        color: candle.close >= candle.open ? 'rgba(16, 185, 129, 0.8)' : 'rgba(248, 113, 113, 0.8)',
      })),
    [candles],
  )

  useEffect(() => {
    if (!chartContainerRef.current) return

    const chart = createChart(chartContainerRef.current, {
      layout: {
        background: { type: ColorType.Solid, color: '#0b1021' },
        textColor: '#d7e2ff',
      },
      grid: {
        vertLines: { color: 'rgba(255,255,255,0.05)' },
        horzLines: { color: 'rgba(255,255,255,0.05)' },
      },
      rightPriceScale: { borderColor: 'rgba(255,255,255,0.1)' },
      timeScale: { borderColor: 'rgba(255,255,255,0.1)' },
      height: 400,
      width: chartContainerRef.current.clientWidth,
    })

    const candleSeries = chart.addSeries(CandlestickSeries, {
      upColor: '#10b981',
      downColor: '#f87171',
      borderVisible: false,
      wickUpColor: '#10b981',
      wickDownColor: '#f87171',
    })

    const volumeSeries = chart.addSeries(HistogramSeries, {
      priceScaleId: 'volume',
      priceFormat: { type: 'volume' },
      priceLineVisible: false,
    })

    chart.applyOptions({
      localization: {
        dateFormat: 'yyyy-MM-dd',
        timeFormatter: (time: UTCTimestamp) => {
          const ts = Number(time) * 1000
          return new Date(Number.isFinite(ts) ? ts : 0).toLocaleTimeString()
        },
      },
    })

    const handleResize = () => {
      if (!chartContainerRef.current) return
      chart.applyOptions({ width: chartContainerRef.current.clientWidth })
    }

    const observer = new ResizeObserver(handleResize)
    observer.observe(chartContainerRef.current)

    candleSeriesRef.current = candleSeries
    volumeSeriesRef.current = volumeSeries

    return () => {
      observer.disconnect()
      chart.remove()
      candleSeriesRef.current = null
      volumeSeriesRef.current = null
    }
  }, [])

  useEffect(() => {
    candleSeriesRef.current?.setData(candles)
    volumeSeriesRef.current?.setData(volumeBars)
  }, [candles, volumeBars])

  useEffect(() => {
    if (!isPlaying) {
      setConnectionStatus('paused')
      return
    }

    const token = (import.meta.env.VITE_AUTH_TOKEN as string | undefined) || undefined
    const ws = new WebSocket(getWsUrl('/ws/trades', token))
    let retryHandle: number | undefined

    ws.onopen = () => setConnectionStatus('live')
    ws.onerror = () => setConnectionStatus('error')
    ws.onmessage = (event) => {
      try {
        const payload = JSON.parse(event.data)
        if (payload?.type !== 'trade') return
        const trade = normalizeTrade(payload.data)
        setTrades((prev) => [...prev.slice(-399), trade])
      } catch (err) {
        console.error('failed to parse trade payload', err)
      }
    }
    ws.onclose = () => {
      if (isPlaying) {
        setConnectionStatus('paused')
        retryHandle = window.setTimeout(() => setReconnectToken((n) => n + 1), 1250)
      }
    }

    return () => {
      if (retryHandle) window.clearTimeout(retryHandle)
      ws.close()
    }
  }, [isPlaying, reconnectToken])

  useEffect(() => {
    if (!tradeTapeRef.current || !isPlaying) return
    tradeTapeRef.current.scrollTo({ top: tradeTapeRef.current.scrollHeight, behavior: 'smooth' })
  }, [filteredTrades, isPlaying])

  const lastTrade = filteredTrades.length > 0 ? filteredTrades[filteredTrades.length - 1] : undefined
  const statusLabel =
    connectionStatus === 'live'
      ? 'Live'
      : connectionStatus === 'connecting'
        ? 'Connecting'
        : connectionStatus === 'error'
          ? 'Error'
          : 'Paused'

  return (
    <div className="page">
      <header className="page__header">
        <div>
          <p className="eyebrow">Limitless matching engine</p>
          <h1>Realtime trade room</h1>
          <p className="muted">Streamed trades feed candles, a fast tape, and simple bot controls.</p>
        </div>
        <div className="controls">
          <button
            className={`control-btn ${isPlaying ? 'control-btn--secondary' : 'control-btn--primary'}`}
            onClick={() => setIsPlaying((v) => !v)}
          >
            {isPlaying ? 'Pause stream' : 'Resume stream'}
          </button>
          <div className={`status status--${connectionStatus}`}>{statusLabel}</div>
        </div>
      </header>

      <section className="panel">
        <div className="panel__header">
          <div>
            <p className="eyebrow">Candles</p>
            <h2>OHLCV from streamed trades</h2>
          </div>
          <div className="chip-row">
            <span className="chip">Interval: {CANDLE_INTERVAL_SECONDS}s</span>
            {lastTrade && <span className="chip">Last trade {lastTrade.symbol}</span>}
          </div>
        </div>
        <div ref={chartContainerRef} className="chart" role="img" aria-label="Candlestick chart" />
      </section>

      <section className="split">
        <div className="panel">
          <div className="panel__header">
            <div>
              <p className="eyebrow">Trade tape</p>
              <h2>Fast-scrolling fills</h2>
            </div>
            <div className="chip-row">
              <span className="chip">Recent trades: {filteredTrades.length}</span>
              {lastTrade && (
                <span className="chip chip--accent">
                  Last price {lastTrade.price.toLocaleString(undefined, { maximumFractionDigits: 4 })}
                </span>
              )}
            </div>
          </div>
          <div ref={tradeTapeRef} className="tape" aria-label="Trade tape">
            {filteredTrades.length === 0 && <div className="empty">Waiting for trades…</div>}
            {filteredTrades.map((trade, idx) => {
              const isBidAggressor = idx % 2 === 0
              return (
                <div key={`${trade.buyOrderId}-${trade.sellOrderId}-${idx}`} className={`tape__row ${isBidAggressor ? 'tape__row--bid' : 'tape__row--ask'}`}>
                  <div className="tape__cell tape__time">
                    {new Date(trade.executedAt).toLocaleTimeString([], { hour12: false })}
                  </div>
                  <div className="tape__cell tape__price">{trade.price.toLocaleString()}</div>
                  <div className="tape__cell tape__qty">{trade.quantity.toLocaleString()}</div>
                  <div className="tape__ids">
                    <span className="pill">{trade.buyOrderId || 'buy'}</span>
                    <span className="pill pill--light">{trade.sellOrderId || 'sell'}</span>
                  </div>
                </div>
              )
            })}
          </div>
        </div>
        <div className="panel panel--compact">
          <div className="panel__header">
            <div>
              <p className="eyebrow">Controls</p>
              <h2>Bot toggles</h2>
            </div>
          </div>
          <div className="control-card">
            <div className="toggle-row">
              <div>
                <p className="toggle__title">Maker bot</p>
                <p className="muted">Hide trades tagged with maker order IDs.</p>
              </div>
              <label className="switch">
                <input
                  type="checkbox"
                  checked={botVisibility.maker}
                  onChange={(e) => setBotVisibility((prev) => ({ ...prev, maker: e.target.checked }))}
                />
                <span className="slider" />
              </label>
            </div>
            <div className="toggle-row">
              <div>
                <p className="toggle__title">Taker bot</p>
                <p className="muted">Toggle trades marked with taker IDs.</p>
              </div>
              <label className="switch">
                <input
                  type="checkbox"
                  checked={botVisibility.taker}
                  onChange={(e) => setBotVisibility((prev) => ({ ...prev, taker: e.target.checked }))}
                />
                <span className="slider" />
              </label>
            </div>
            <p className="muted small">
              Streams use the configured backend WebSocket (see README) and replay into the chart and tape. Pause
              streaming to inspect the most recent batch without new updates.
            </p>
          </div>
        </div>
      </section>
    </div>
  )
}

export default App
