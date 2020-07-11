package strategy

import (
	"context"
	"fmt"
	"github.com/c9s/bbgo/pkg/slack/slackstyle"
	"math"
	"strconv"
	"time"

	"github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/bbgo/exchange/binance"
	"github.com/c9s/bbgo/pkg/bbgo/types"
	"github.com/c9s/bbgo/pkg/util"
	"github.com/slack-go/slack"
)

type KLineStrategy struct {
	Symbol    string          `json:"symbol"`
	Detectors []KLineDetector `json:"detectors"`

	Trader          *bbgo.Trader                 `json:"-"`
	KLineWindowSize int                          `json:"-"`
	KLineWindows    map[string]types.KLineWindow `json:"-"`
	cache           *util.VolatileMemory         `json:"-"`

	price60daysLow  float64
	price60daysHigh float64
}

func (s *KLineStrategy) Init(ctx context.Context, trader *bbgo.Trader, exchange *binance.Exchange) error {
	s.Trader = trader
	s.cache = util.NewDetectorCache()
	s.KLineWindows = map[string]types.KLineWindow{}

	for _, interval := range []string{"1m", "5m", "1h", "1d"} {
		klines, err := exchange.QueryKLines(ctx, s.Symbol, interval, s.KLineWindowSize)
		if err != nil {
			return err
		}

		s.KLineWindows[interval] = klines
	}

	kline60days := s.KLineWindows["1d"].Take(60)
	s.price60daysLow = kline60days.GetLow()
	s.price60daysHigh = kline60days.GetHigh()
	return nil
}

// Subscribe defines what to subscribe for the strategy
func (s *KLineStrategy) OnConnect(stream *binance.PrivateStream) {
	stream.Subscribe("kline", s.Symbol, binance.SubscribeOptions{Interval: "1m"})
	stream.Subscribe("kline", s.Symbol, binance.SubscribeOptions{Interval: "5m"})
	stream.Subscribe("kline", s.Symbol, binance.SubscribeOptions{Interval: "1h"})
	stream.Subscribe("kline", s.Symbol, binance.SubscribeOptions{Interval: "1d"})
}

func (s *KLineStrategy) OnKLineClosedEvent(e *binance.KLineEvent) {
	s.AddKLine(*e.KLine)

	trend := e.KLine.GetTrend()

	// price is not changed, do not act
	if trend == 0 {
		return
	}

	var trendIcon = slackstyle.TrendIcon(trend)
	var trader = s.Trader
	var ctx = context.Background()

	for _, detector := range s.Detectors {
		if detector.Interval != e.KLine.Interval {
			continue
		}

		reason, kline, ok := detector.Detect(e, trader.Context, s)
		if !ok {

			if len(reason) > 0 &&
				(s.cache.IsTextFresh(reason, 30*time.Minute) &&
					s.cache.IsObjectFresh(&detector, 10*time.Minute)) {
				trader.Infof(trendIcon+" *SKIP* reason: %s", reason, detector.SlackAttachment(), slackstyle.SlackAttachmentCreator(kline).SlackAttachment())
			}

		} else {
			if len(reason) > 0 {
				trader.Infof(trendIcon+" *TRIGGERED* reason: %s", reason, detector.SlackAttachment(), slackstyle.SlackAttachmentCreator(kline).SlackAttachment())
			} else {
				trader.Infof(trendIcon+" *TRIGGERED* ", detector.SlackAttachment(), slackstyle.SlackAttachmentCreator(kline).SlackAttachment())
			}

			var order = detector.NewOrder(e, trader.Context, s)

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

func (s *KLineStrategy) AddKLine(kline types.KLine) types.KLineWindow {
	var klineWindow = s.KLineWindows[kline.Interval]
	klineWindow.Add(kline)

	if s.KLineWindowSize > 0 {
		klineWindow.Truncate(s.KLineWindowSize)
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
	name += " MaxMaxPriceChange " + maxPriceChangeRange

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

func (d *KLineDetector) NewOrder(e *binance.KLineEvent, tradingCtx *bbgo.TradingContext, strategy *KLineStrategy) *types.Order {
	var kline types.KLineOrWindow = e.KLine
	if d.EnableLookBack {
		klineWindow := strategy.KLineWindows[e.KLine.Interval]
		if len(klineWindow) >= d.LookBackFrames {
			kline = klineWindow.Tail(d.LookBackFrames)
		}
	}

	var trend = kline.GetTrend()

	var side types.SideType
	if trend < 0 {
		side = types.SideTypeBuy
	} else if trend > 0 {
		side = types.SideTypeSell
	}

	var volume = tradingCtx.Market.FormatVolume(VolumeByPriceChange(tradingCtx.Market, kline.GetClose(), kline.GetChange(), side))
	return &types.Order{
		Symbol:    e.KLine.Symbol,
		Type:      types.OrderTypeMarket,
		Side:      side,
		VolumeStr: volume,
	}
}

func (d *KLineDetector) Detect(e *binance.KLineEvent, tradingCtx *bbgo.TradingContext, strategy *KLineStrategy) (reason string, kline types.KLineOrWindow, ok bool) {
	kline = e.KLine

	// if the 3m trend is drop, do not buy, let 5m window handle it.
	if d.EnableLookBack {
		klineWindow := strategy.KLineWindows[e.KLine.Interval]
		if len(klineWindow) >= d.LookBackFrames {
			kline = klineWindow.Tail(d.LookBackFrames)
		}
		/*
			if lookbackKline.AllDrop() {
				trader.Infof("1m window all drop down (%d frames), do not buy: %+v", d.LookBackFrames, klineWindow)
			} else if lookbackKline.AllRise() {
				trader.Infof("1m window all rise up (%d frames), do not sell: %+v", d.LookBackFrames, klineWindow)
			}
		*/
	}

	var maxChange = math.Abs(kline.GetMaxChange())

	if maxChange < d.MinMaxPriceChange {
		return "", kline, false
	}

	if util.NotZero(d.MaxMaxPriceChange) && maxChange > d.MaxMaxPriceChange {
		return fmt.Sprintf("exceeded max price change %.4f > %.4f", maxChange, d.MaxMaxPriceChange), kline, false
	}

	if d.EnableMinThickness {
		if kline.GetThickness() < d.MinThickness {
			return fmt.Sprintf("kline too thin. %.4f < min kline thickness %.4f", kline.GetThickness(), d.MinThickness), kline, false
		}
	}

	var trend = kline.GetTrend()
	if d.EnableMaxShadowRatio {
		if trend > 0 {
			if kline.GetUpperShadowRatio() > d.MaxShadowRatio {
				return fmt.Sprintf("kline upper shadow ratio too high. %.4f > %.4f (MaxShadowRatio)", kline.GetUpperShadowRatio(), d.MaxShadowRatio), kline, false
			}
		} else if trend < 0 {
			if kline.GetLowerShadowRatio() > d.MaxShadowRatio {
				return fmt.Sprintf("kline lower shadow ratio too high. %.4f > %.4f (MaxShadowRatio)", kline.GetLowerShadowRatio(), d.MaxShadowRatio), kline, false
			}
		}
	}

	if trend > 0 && kline.BounceUp() { // trend up, ignore bounce up

		return fmt.Sprintf("bounce up, do not sell, kline mid: %.4f", kline.Mid()), kline, false

	} else if trend < 0 && kline.BounceDown() { // trend down, ignore bounce down

		return fmt.Sprintf("bounce down, do not buy, kline mid: %.4f", kline.Mid()), kline, false

	}

	if util.NotZero(d.MinProfitPriceTick) {

		// do not buy too early if it's greater than the average bid price + min profit tick
		if trend < 0 && kline.GetClose() > (tradingCtx.AverageBidPrice-d.MinProfitPriceTick) {
			return fmt.Sprintf("price %f is greater than the average price + min profit tick %f", kline.GetClose(), tradingCtx.AverageBidPrice-d.MinProfitPriceTick), kline, false
		}

		// do not sell too early if it's less than the average bid price + min profit tick
		if trend > 0 && kline.GetClose() < (tradingCtx.AverageBidPrice+d.MinProfitPriceTick) {
			return fmt.Sprintf("price %f is less than the average price + min profit tick %f", kline.GetClose(), tradingCtx.AverageBidPrice+d.MinProfitPriceTick), kline, false
		}

	}

	/*
			if toPrice(kline.GetClose()) == toPrice(kline.GetLow()) {
			return fmt.Sprintf("close near the lowest price, the price might continue to drop."), false
		}

	*/

	return "", kline, true
}
