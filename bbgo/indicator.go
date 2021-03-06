package bbgo

import (
	"math"
	"time"

	"github.com/c9s/bbgo/types"
)

type MovingAverageIndicator struct {
	store *MarketDataStore
	Period int
}

func NewMovingAverageIndicator(period int) *MovingAverageIndicator {
	return &MovingAverageIndicator{
		Period: period,
	}
}

func (i *MovingAverageIndicator) handleUpdate(kline types.KLine) {
	klines, ok := i.store.KLineWindows[ Interval(kline.Interval) ]
	if !ok {
		return
	}

	if len(klines) < i.Period {
		return
	}

	// calculate ma
}

type IndicatorValue struct {
	Value float64
	Time time.Time
}

func calculateMovingAverage(klines types.KLineWindow, period int) (values []IndicatorValue) {
	for idx := range klines[period:] {
		offset := idx + period
		sum := klines[offset - period:offset].ReduceClose()
		values = append(values, IndicatorValue{
			Time:  klines[offset].GetEndTime(),
			Value: math.Round(sum / float64(period)),
		})
	}
	return values
}




func (i *MovingAverageIndicator) SubscribeStore(store *MarketDataStore) {
	i.store = store

	// register kline update callback
	store.OnUpdate(i.handleUpdate)
}




