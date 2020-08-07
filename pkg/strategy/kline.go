package strategy

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/slack-go/slack"

	"github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/bbgo/types"
	"github.com/c9s/bbgo/pkg/slack/slackstyle"
	"github.com/c9s/bbgo/pkg/util"
)

//go:generate callbackgen -type KLineStrategy
type KLineStrategy struct {
	Symbol          string          `json:"symbol"`
	Detectors       []KLineDetector `json:"detectors"`
	BaseQuantity    float64         `json:"baseQuantity"`
	KLineWindowSize int             `json:"kLineWindowSize"`

	MinProfitSpread float64 `json:"minProfitSpread"`
	StopBuyRatio    float64 `json:"stopBuyRatio"`
	StopSellRatio   float64 `json:"stopSellRatio"`

	// runtime variables
	Trader         types.Trader         `json:"-"`
	TradingContext *bbgo.TradingContext `json:"-"`

	Notifier bbgo.Notifier `json:"-"`

	market           types.Market                 `json:"-"`
	KLineWindows     map[string]types.KLineWindow `json:"-"`
	cache            *util.VolatileMemory         `json:"-"`
	volumeCalculator *VolumeCalculator            `json:"-"`

	detectCallbacks []func(ok bool, reason string, detector *KLineDetector, kline types.KLineOrWindow)
}

func (strategy *KLineStrategy) Init(tradingContext *bbgo.TradingContext, trader types.Trader) error {
	strategy.TradingContext = tradingContext
	strategy.Trader = trader
	strategy.cache = util.NewDetectorCache()

	market, ok := types.FindMarket(strategy.Symbol)
	if !ok {
		return fmt.Errorf("market not found %s", strategy.Symbol)
	}

	strategy.market = market

	days := 7
	klineWindow := strategy.KLineWindows["1d"].Tail(7)
	high := klineWindow.GetHigh()
	low := klineWindow.GetLow()

	strategy.Notifier.Notify("Here is the historical price for strategy: high / low %f / %f", high, low, slack.Attachment{
		Title:   fmt.Sprintf("%s Historical Price %d days high and low", strategy.Symbol, days),
		Pretext: "",
		Text:    "",
		Fields: []slack.AttachmentField{
			{Title: "High", Value: market.FormatPrice(high), Short: true},
			{Title: "Low", Value: market.FormatPrice(low), Short: true},
			{Title: "Current", Value: market.FormatPrice(klineWindow.GetClose()), Short: true},
		},
	})

	strategy.volumeCalculator = &VolumeCalculator{
		Market:         market,
		BaseQuantity:   strategy.BaseQuantity,
		HistoricalHigh: high,
		HistoricalLow:  low,
	}

	return nil
}

func (strategy *KLineStrategy) OnNewStream(stream *types.StandardPrivateStream) error {
	// configure stream handlers
	stream.Subscribe("kline", strategy.Symbol, types.SubscribeOptions{Interval: "1m"})
	stream.Subscribe("kline", strategy.Symbol, types.SubscribeOptions{Interval: "5m"})
	stream.Subscribe("kline", strategy.Symbol, types.SubscribeOptions{Interval: "1h"})
	stream.Subscribe("kline", strategy.Symbol, types.SubscribeOptions{Interval: "1d"})
	stream.OnKLineClosed(strategy.OnKLineClosed)
	return nil
}

func (strategy *KLineStrategy) OnKLineClosed(kline *types.KLine) {
	strategy.AddKLine(*kline)

	trend := kline.GetTrend()

	// price is not changed, do not act
	if trend == 0 {
		return
	}

	var trendIcon = slackstyle.TrendIcon(trend)
	var ctx = context.Background()

	for _, detector := range strategy.Detectors {
		if detector.Interval != kline.Interval {
			continue
		}

		var klineOrWindow types.KLineOrWindow = kline
		if detector.EnableLookBack {
			klineWindow := strategy.KLineWindows[kline.Interval]
			if len(klineWindow) >= detector.LookBackFrames {
				klineOrWindow = klineWindow.Tail(detector.LookBackFrames)
			}
		}

		reason, ok := detector.Detect(klineOrWindow, strategy.TradingContext)
		strategy.EmitDetect(ok, reason, &detector, klineOrWindow)

		if !ok {

			if len(reason) > 0 &&
				(strategy.cache.IsTextFresh(reason, 30*time.Minute) &&
					strategy.cache.IsObjectFresh(&detector, 10*time.Minute)) {

				strategy.Notifier.Notify(trendIcon+" *SKIP* reason: %s", reason, &detector, kline)
			}

		} else {
			if len(reason) > 0 {
				strategy.Notifier.Notify(trendIcon+" *TRIGGERED* reason: %s", reason, &detector, kline)
			} else {
				strategy.Notifier.Notify(trendIcon+" *TRIGGERED* ", &detector, kline)
			}

			var order, err = strategy.NewOrder(klineOrWindow, strategy.TradingContext)
			if err != nil {
				strategy.Notifier.Notify("%s order error: %v", kline.Symbol, err)
				return
			}

			if order == nil {
				return
			}

			recentKLines := strategy.KLineWindows[kline.Interval]
			switch kline.Interval {
			case "1m":
				recentKLines = recentKLines.Tail(60 * 8) // 8 hours
			case "5m":
				recentKLines = recentKLines.Tail(12 * 5 * 8) // 8 hours
			case "1h":
				recentKLines = recentKLines.Tail(1 * 8) // 8 hours
			case "1d":
				recentKLines = recentKLines.Tail(1 * 7)

			default:
				recentKLines = recentKLines.Tail(3)

			}

			recentHigh := recentKLines.GetHigh()
			recentLow := recentKLines.GetLow()
			recentChange := recentHigh - recentLow
			closedPrice := kline.GetClose()

			switch order.Side {

			case types.SideTypeSell:
				stopPrice := recentLow + strategy.StopSellRatio*recentChange
				if closedPrice < stopPrice {
					attachment := slack.Attachment{
						Title: "Stop Sell Condition",
						Fields: []slack.AttachmentField{
							{Short: true, Title: "Current Price", Value: strategy.market.FormatPrice(closedPrice)},
							{Short: true, Title: "Stop Price", Value: strategy.market.FormatPrice(stopPrice)},
							{Short: true, Title: "Recent Low", Value: strategy.market.FormatPrice(recentLow)},
							{Short: true, Title: "Ratio", Value: util.FormatFloat(strategy.StopSellRatio, 2)},
							{Short: false, Title: "Recent Max Price Change", Value: util.FormatFloat(recentChange, 2)},
						},
					}
					strategy.Notifier.Notify(":raised_hands: %s stop sell", kline.Symbol, attachment, recentKLines)
					return
				}

			case types.SideTypeBuy:
				stopPrice := recentHigh - strategy.StopBuyRatio*recentChange
				if closedPrice > stopPrice {
					attachment := slack.Attachment{
						Title: "Stop Buy Condition",
						Fields: []slack.AttachmentField{
							{Short: true, Title: "Current Price", Value: strategy.market.FormatPrice(closedPrice)},
							{Short: true, Title: "Stop Price", Value: strategy.market.FormatPrice(stopPrice)},
							{Short: true, Title: "Recent High", Value: strategy.market.FormatPrice(recentHigh)},
							{Short: true, Title: "Ratio", Value: util.FormatFloat(strategy.StopBuyRatio, 2)},
							{Short: false, Title: "Recent Max Price Change", Value: util.FormatFloat(recentChange, 2)},
						},
					}
					strategy.Notifier.Notify(":raised_hands: %s stop buy", kline.Symbol, attachment, recentKLines)
					return
				}

			}

			strategy.Trader.SubmitOrder(ctx, order)

			if detector.Stop {
				return
			}
		}
	}
}

func (strategy *KLineStrategy) NewOrder(kline types.KLineOrWindow, tradingCtx *bbgo.TradingContext) (*types.Order, error) {
	var trend = kline.GetTrend()

	var side types.SideType
	if trend < 0 {
		side = types.SideTypeBuy
	} else if trend > 0 {
		side = types.SideTypeSell
	}

	var currentPrice = kline.GetClose()
	var quantity = strategy.volumeCalculator.Volume(currentPrice, kline.GetChange(), side)

	tradingCtx.Lock()
	defer tradingCtx.Unlock()

	switch side {
	case types.SideTypeBuy:

		if balance, ok := tradingCtx.Balances[strategy.market.QuoteCurrency]; ok {
			if balance.Available < 2000.0 {
				return nil, fmt.Errorf("quote balance level is too low: %s", bbgo.USD.FormatMoneyFloat64(balance.Available))
			}

			available := math.Max(0.0, balance.Available - 2000.0)

			if available < strategy.market.MinAmount {
				return nil, fmt.Errorf("insufficient quote balance: %f < min amount %f", available, strategy.market.MinAmount)
			}

			quantity = adjustVolumeByMinAmount(quantity, currentPrice, strategy.market.MinAmount * 1.1)
			quantity = adjustVolumeByMaxAmount(quantity, currentPrice, available)
			amount := quantity * currentPrice
			if amount < strategy.market.MinAmount {
				return nil, fmt.Errorf("amount too small: %f < min amount %f", amount, strategy.market.MinAmount)
			}
		}

	case types.SideTypeSell:

		if balance, ok := tradingCtx.Balances[strategy.market.BaseCurrency]; ok {
			available := balance.Available

			if available < strategy.market.MinQuantity {
				return nil, fmt.Errorf("insufficient base balance: %f > minimal quantity %f", available, strategy.market.MinQuantity)
			}

			quantity = math.Min(quantity, available)

			// price tick10
			// 2 -> 0.01 -> 0.1
			// 4 -> 0.0001 -> 0.001
			tick10 := math.Pow10(-strategy.market.PricePrecision + 1)
			minProfitSpread := math.Max(strategy.MinProfitSpread, tick10)
			estimatedFee := currentPrice * 0.001
			targetPrice := currentPrice - estimatedFee - minProfitSpread

			stockQuantity := strategy.TradingContext.StockManager.Stocks.QuantityBelowPrice(targetPrice)
			if math.Round(stockQuantity*1e8) == 0.0 {
				return nil, fmt.Errorf("profitable stock not found: target price %f, profit spread: %f", targetPrice, minProfitSpread)
			}

			quantity = math.Min(quantity, stockQuantity)
			if quantity < tradingCtx.Market.MinLot {
				return nil, fmt.Errorf("quantity %f less than min lot %f", quantity, tradingCtx.Market.MinLot)
			}

			notional := quantity * currentPrice
			if notional < tradingCtx.Market.MinNotional {
				return nil, fmt.Errorf("notional %f < min notional: %f", notional , tradingCtx.Market.MinNotional)
			}

		}
	}

	return &types.Order{
		Symbol:    strategy.Symbol,
		Type:      types.OrderTypeMarket,
		Side:      side,
		VolumeStr: tradingCtx.Market.FormatVolume(quantity),
	}, nil
}

func (strategy *KLineStrategy) AddKLine(kline types.KLine) types.KLineWindow {
	var klineWindow = strategy.KLineWindows[kline.Interval]
	klineWindow.Add(kline)

	if strategy.KLineWindowSize > 0 {
		klineWindow.Truncate(strategy.KLineWindowSize)
	}

	strategy.KLineWindows[kline.Interval] = klineWindow

	strategy.volumeCalculator.HistoricalHigh = math.Max(strategy.volumeCalculator.HistoricalHigh, kline.GetHigh())
	strategy.volumeCalculator.HistoricalLow = math.Min(strategy.volumeCalculator.HistoricalLow, kline.GetLow())
	return klineWindow
}

type KLineDetector struct {
	Name     string `json:"name"`
	Interval string `json:"interval"`

	// MinMaxPriceChange is the minimal max price change trigger
	MinMaxPriceChange float64 `json:"minMaxPriceChange"`

	// MaxMaxPriceChange is the max - max price change trigger
	MaxMaxPriceChange float64 `json:"maxMaxPriceChange"`

	EnableMinThickness bool    `json:"enableMinThickness"`
	MinThickness       float64 `json:"minThickness"`

	EnableMaxShadowRatio bool    `json:"enableMaxShadowRatio"`
	MaxShadowRatio       float64 `json:"maxShadowRatio"`

	EnableLookBack bool `json:"enableLookBack"`
	LookBackFrames int  `json:"lookBackFrames"`

	MinProfitPriceTick float64 `json:"minProfitPriceTick"`

	DelayMilliseconds int  `json:"delayMsec"`
	Stop              bool `json:"stop"`
}

func (d *KLineDetector) SlackAttachment() slack.Attachment {
	var name = "Detector "

	if len(d.Name) > 0 {
		name += " " + d.Name
	}

	name += fmt.Sprintf(" %s", d.Interval)

	if d.EnableLookBack {
		name += fmt.Sprintf(" x %d", d.LookBackFrames)
	}

	var maxPriceChangeRange = fmt.Sprintf("%.2f ~ NO LIMIT", d.MinMaxPriceChange)
	if util.NotZero(d.MaxMaxPriceChange) {
		maxPriceChangeRange = fmt.Sprintf("%.2f ~ %.2f", d.MinMaxPriceChange, d.MaxMaxPriceChange)
	}
	name += " MaxPriceChangeRange " + maxPriceChangeRange

	var fields = []slack.AttachmentField{
		{
			Title: "Interval",
			Value: d.Interval,
			Short: true,
		},
	}

	if d.EnableMinThickness && util.NotZero(d.MinThickness) {
		fields = append(fields, slack.AttachmentField{
			Title: "MinThickness",
			Value: util.FormatFloat(d.MinThickness, 4),
			Short: true,
		})
	}

	if d.EnableMaxShadowRatio && util.NotZero(d.MaxShadowRatio) {
		fields = append(fields, slack.AttachmentField{
			Title: "MaxShadowRatio",
			Value: util.FormatFloat(d.MaxShadowRatio, 4),
			Short: true,
		})
	}

	if d.EnableLookBack {
		fields = append(fields, slack.AttachmentField{
			Title: "LookBackFrames",
			Value: strconv.Itoa(d.LookBackFrames),
			Short: true,
		})
	}

	return slack.Attachment{
		Color:      "",
		Fallback:   "",
		ID:         0,
		Title:      name,
		Pretext:    "",
		Text:       "",
		Fields:     fields,
		Footer:     "",
		FooterIcon: "",
		Ts:         "",
	}

}

func (d *KLineDetector) String() string {
	var name = fmt.Sprintf("Detector %s (%f < x < %f)", d.Interval, d.MinMaxPriceChange, d.MaxMaxPriceChange)

	if d.EnableMinThickness {
		name += fmt.Sprintf(" [MinThickness: %f]", d.MinThickness)
	}

	if d.EnableLookBack {
		name += fmt.Sprintf(" [LookBack: %d]", d.LookBackFrames)
	}
	if d.EnableMaxShadowRatio {
		name += fmt.Sprintf(" [MaxShadowRatio: %f]", d.MaxShadowRatio)
	}

	return name
}

func (d *KLineDetector) Detect(kline types.KLineOrWindow, tradingCtx *bbgo.TradingContext) (reason string, ok bool) {
	/*
		if lookbackKline.AllDrop() {
			Trader.Notify("1m window all drop down (%d frames), do not buy: %+v", d.LookBackFrames, klineWindow)
		} else if lookbackKline.AllRise() {
			Trader.Notify("1m window all rise up (%d frames), do not sell: %+v", d.LookBackFrames, klineWindow)
		}
	*/

	var maxChange = math.Abs(kline.GetMaxChange())

	if maxChange < d.MinMaxPriceChange {
		return "", false
	}

	if util.NotZero(d.MaxMaxPriceChange) && maxChange > d.MaxMaxPriceChange {
		return fmt.Sprintf("exceeded max price change %.4f > %.4f", maxChange, d.MaxMaxPriceChange), false
	}

	if d.EnableMinThickness {
		if kline.GetThickness() < d.MinThickness {
			return fmt.Sprintf("kline too thin. %.4f < min kline thickness %.4f", kline.GetThickness(), d.MinThickness), false
		}
	}

	var trend = kline.GetTrend()
	if d.EnableMaxShadowRatio {
		if trend > 0 {
			if kline.GetUpperShadowRatio() > d.MaxShadowRatio {
				return fmt.Sprintf("kline upper shadow ratio too high. %.4f > %.4f (MaxShadowRatio)", kline.GetUpperShadowRatio(), d.MaxShadowRatio), false
			}
		} else if trend < 0 {
			if kline.GetLowerShadowRatio() > d.MaxShadowRatio {
				return fmt.Sprintf("kline lower shadow ratio too high. %.4f > %.4f (MaxShadowRatio)", kline.GetLowerShadowRatio(), d.MaxShadowRatio), false
			}
		}
	}

	if trend > 0 && kline.BounceUp() { // trend up, ignore bounce up

		return fmt.Sprintf("bounce up, do not sell, kline mid: %.4f", kline.Mid()), false

	} else if trend < 0 && kline.BounceDown() { // trend down, ignore bounce down

		return fmt.Sprintf("bounce down, do not buy, kline mid: %.4f", kline.Mid()), false

	}

	if util.NotZero(d.MinProfitPriceTick) {

		// do not buy too early if it's greater than the average bid price + min profit tick
		if trend < 0 && kline.GetClose() > (tradingCtx.AverageBidPrice-d.MinProfitPriceTick) {
			return fmt.Sprintf("price %f is greater than the average price + min profit tick %f", kline.GetClose(), tradingCtx.AverageBidPrice-d.MinProfitPriceTick), false
		}

		// do not sell too early if it's less than the average bid price + min profit tick
		if trend > 0 && kline.GetClose() < (tradingCtx.AverageBidPrice+d.MinProfitPriceTick) {
			return fmt.Sprintf("price %f is less than the average price + min profit tick %f", kline.GetClose(), tradingCtx.AverageBidPrice+d.MinProfitPriceTick), false
		}

	}

	/*
			if toPrice(kline.GetClose()) == toPrice(kline.GetLow()) {
			return fmt.Sprintf("close near the lowest price, the price might continue to drop."), false
		}

	*/

	return "", true
}
