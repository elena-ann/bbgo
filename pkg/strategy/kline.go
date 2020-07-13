package strategy

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/slack-go/slack"

	"github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/bbgo/exchange/binance"
	"github.com/c9s/bbgo/pkg/bbgo/types"
	"github.com/c9s/bbgo/pkg/slack/slackstyle"
	"github.com/c9s/bbgo/pkg/util"
)

type KLineService interface {
	QueryKLines(ctx context.Context, symbol string, interval string, limit int) ([]types.KLine, error)
}

//go:generate callbackgen -type KLineStrategy
type KLineStrategy struct {
	Symbol          string          `json:"symbol"`
	Detectors       []KLineDetector `json:"detectors"`
	BaseQuantity    float64         `json:"baseQuantity"`
	KLineWindowSize int             `json:"kLineWindowSize"`

	// runtime variables
	Trader   *bbgo.Trader `json:"-"`
	Notifier *bbgo.SlackNotifier `json:"-"`

	market           types.Market                 `json:"-"`
	KLineWindows     map[string]types.KLineWindow `json:"-"`
	cache            *util.VolatileMemory         `json:"-"`
	volumeCalculator *VolumeCalculator            `json:"-"`

	detectCallbacks []func(ok bool, reason string, detector *KLineDetector, kline types.KLineOrWindow)
}

func (strategy *KLineStrategy) Init(trader *bbgo.Trader) error {
	strategy.Trader = trader
	strategy.cache = util.NewDetectorCache()

	market, ok := types.FindMarket(strategy.Symbol)
	if !ok {
		return fmt.Errorf("market not found %s", strategy.Symbol)
	}

	strategy.market = market

	klineWindow := strategy.KLineWindows["1d"].Tail(60)
	strategy.volumeCalculator = &VolumeCalculator{
		Market:         market,
		BaseQuantity:   strategy.BaseQuantity,
		HistoricalHigh: klineWindow.GetHigh(),
		HistoricalLow:  klineWindow.GetLow(),
	}

	return nil
}

func (strategy *KLineStrategy) OnNewStream(stream *binance.PrivateStream) error {
	// configure stream handlers
	stream.OnConnect(strategy.OnConnect)
	stream.OnKLineClosedEvent(strategy.OnKLineClosedEvent)
	stream.Subscribe("kline", strategy.Symbol, binance.SubscribeOptions{Interval: "1m"})
	stream.Subscribe("kline", strategy.Symbol, binance.SubscribeOptions{Interval: "5m"})
	stream.Subscribe("kline", strategy.Symbol, binance.SubscribeOptions{Interval: "1h"})
	stream.Subscribe("kline", strategy.Symbol, binance.SubscribeOptions{Interval: "1d"})
	return nil
}


// Subscribe defines what to subscribe for the strategy
func (strategy *KLineStrategy) OnConnect(stream *binance.PrivateStream) {
}

func (strategy *KLineStrategy) OnKLineClosedEvent(e *binance.KLineEvent) {
	strategy.AddKLine(*e.KLine)

	trend := e.KLine.GetTrend()

	// price is not changed, do not act
	if trend == 0 {
		return
	}

	var trendIcon = slackstyle.TrendIcon(trend)
	var trader = strategy.Trader
	var ctx = context.Background()

	for _, detector := range strategy.Detectors {
		if detector.Interval != e.KLine.Interval {
			continue
		}

		var kline types.KLineOrWindow = e.KLine
		if detector.EnableLookBack {
			klineWindow := strategy.KLineWindows[e.KLine.Interval]
			if len(klineWindow) >= detector.LookBackFrames {
				kline = klineWindow.Tail(detector.LookBackFrames)
			}
		}

		reason, ok := detector.Detect(kline, trader.Context)

		strategy.EmitDetect(ok, reason, &detector, kline)

		if !ok {

			if len(reason) > 0 &&
				(strategy.cache.IsTextFresh(reason, 30*time.Minute) &&
					strategy.cache.IsObjectFresh(&detector, 10*time.Minute)) {

				strategy.Notifier.Notify(trendIcon+" *SKIP* reason: %s", reason, detector.SlackAttachment(), slackstyle.SlackAttachmentCreator(kline).SlackAttachment())
			}

		} else {
			if len(reason) > 0 {
				strategy.Notifier.Notify(trendIcon+" *TRIGGERED* reason: %s", reason, detector.SlackAttachment(), slackstyle.SlackAttachmentCreator(kline).SlackAttachment())
			} else {
				strategy.Notifier.Notify(trendIcon+" *TRIGGERED* ", detector.SlackAttachment(), slackstyle.SlackAttachmentCreator(kline).SlackAttachment())
			}

			var order = strategy.NewOrder(kline, trader.Context)
			if order != nil {
				var delay = time.Duration(detector.DelayMilliseconds) * time.Millisecond

				// add a delay
				if delay > 0 {
					time.AfterFunc(delay, func() {
						trader.SubmitOrder(ctx, order)
					})
				} else {
					trader.SubmitOrder(ctx, order)
				}
			}

			if detector.Stop {
				return
			}
		}
	}
}

func (strategy *KLineStrategy) NewOrder(kline types.KLineOrWindow, tradingCtx *bbgo.TradingContext) *types.Order {
	var trend = kline.GetTrend()

	var side types.SideType
	if trend < 0 {
		side = types.SideTypeBuy
	} else if trend > 0 {
		side = types.SideTypeSell
	}

	var v = strategy.volumeCalculator.Volume(kline.GetClose(), kline.GetChange(), side)
	var volume = tradingCtx.Market.FormatVolume(v)
	return &types.Order{
		Symbol:    strategy.Symbol,
		Type:      types.OrderTypeMarket,
		Side:      side,
		VolumeStr: volume,
	}
}

func (strategy *KLineStrategy) AddKLine(kline types.KLine) types.KLineWindow {
	var klineWindow = strategy.KLineWindows[kline.Interval]
	klineWindow.Add(kline)

	if strategy.KLineWindowSize > 0 {
		klineWindow.Truncate(strategy.KLineWindowSize)
	}

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
