package strategy

var DefaultKLineStrategy = KLineStrategy{
	Symbol:         "BTCUSDT",
	StopWindowSize: 500,
	BaseQuantity:   0.1,
	Detectors: []KLineDetector{
		// extremely short term rules

		// 1m quick drops or raises
		{
			Interval:          "1m",
			MinMaxPriceChange: 42.0,

			EnableMinThickness: true,
			MinThickness:       5.5 / 10.0,

			EnableMaxShadowRatio: true,
			MaxShadowRatio:       55.0 / 100.0,

			Stop: true,
		},

		// 5m drops or raises
		{
			Interval:          "1m",
			MinMaxPriceChange: 62.0,

			EnableMinThickness: true,
			MinThickness:       5.5 / 10.0,

			EnableMaxShadowRatio: true,
			MaxShadowRatio:       50.0 / 100.0,

			EnableLookBack: true,
			LookBackFrames: 5, // replace 5m interval

			Stop: true,
		},


		// short term rules
		// 45 minutes grid trading, find the lowest price and highest price in the 30 minutes window.
		{
			Interval:          "1m",
			MinMaxPriceChange: 60.0,
			MaxMaxPriceChange: 99.0,

			EnableMinThickness: true,
			MinThickness:       8.0 / 10.0, // 37.5 ~ 90.0

			EnableMaxShadowRatio: true,
			MaxShadowRatio:       3.2 / 10.0,

			EnableLookBack: true,
			LookBackFrames: 45,
		},


		// middle-term rule
		{
			Interval:          "1h",
			MinMaxPriceChange: 119.0, // 179.0

			EnableMinThickness: true,
			MinThickness:       1.0 / 3.0, // real body should be above 1/3 of the candlestick

			EnableMaxShadowRatio: true,
			MaxShadowRatio:       40.0 / 100.0,
		},

		{
			Interval:          "1d",
			MinMaxPriceChange: 299.0, // 179.0

			EnableMinThickness: true,
			MinThickness:       1.0 / 3.0, // real body should be above 1/3 of the candlestick

			EnableMaxShadowRatio: true,
			MaxShadowRatio:       40.0 / 100.0,
		},
	},
}
