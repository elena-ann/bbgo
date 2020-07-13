package strategy

import (
	"github.com/c9s/bbgo/pkg/bbgo/types"
	"math"
)

// https://www.desmos.com/calculator/wik4ozkwto
type VolumeCalculator struct {
	Market types.Market

	BaseQuantity float64
	HistoricalHigh float64 // 10500.0
	HistoricalLow  float64 // 7500.0
}

func (c *VolumeCalculator) modifyBuyVolume(price float64) float64 {
	maxChange := c.HistoricalHigh - c.HistoricalLow
	pessimisticIndex := 0.9
	targetPrice := c.HistoricalLow * pessimisticIndex // we will get 1 at price 7500, and more below 7500
	flatness := maxChange * 0.5    // higher number buys more in the middle section. higher number gets more flat line, reduced to 0 at price 2000 * 10
	return math.Exp(-(price - targetPrice) / flatness)
}

func (c *VolumeCalculator) modifySellVolume(price float64) float64 {
	// \exp\left(\frac{x-10000}{500}\right)
	maxChange := c.HistoricalHigh - c.HistoricalLow
	optimismIndex := 1.1
	targetPrice := c.HistoricalHigh * optimismIndex // target to sell most x1 at 10000.0
	flatness := maxChange * 0.4     // higher number sells more in the middle section, lower number sells fewer in the middle section.
	return math.Exp((price - targetPrice) / flatness)
}

func (c *VolumeCalculator) VolumeByChange(change float64) float64 {
	maxChange := c.HistoricalHigh - c.HistoricalLow
	flatness := maxChange * 0.6
	return math.Exp((math.Abs(change))/flatness)
}

func (c *VolumeCalculator) minQuantity(volume float64) float64 {
	return math.Max(c.Market.MinQuantity, volume)
}

func mostMaxAmount(volume float64, currentPrice float64, maxAmount float64) float64 {
	// modify volume for the min amount
	amount := currentPrice * volume
	if amount > maxAmount {
		ratio := maxAmount / amount
		volume *= ratio
	}

	return volume
}


func atLeastMinAmount(volume float64, currentPrice float64, minAmount float64) float64 {
	// modify volume for the min amount
	amount := currentPrice * volume
	if amount < minAmount {
		ratio := minAmount / amount
		volume *= ratio
	}

	return volume
}

func (c *VolumeCalculator) Volume(currentPrice float64, change float64, side types.SideType) float64 {
	volume := c.BaseQuantity * c.VolumeByChange(change)

	if side == types.SideTypeSell {
		volume *= c.modifySellVolume(currentPrice)
	} else {
		volume *= c.modifyBuyVolume(currentPrice)
	}

	volume = c.minQuantity(volume)
	volume = atLeastMinAmount(volume, currentPrice, c.Market.MinAmount)
	return c.Market.CanonicalizeVolume(volume)
}

// https://www.desmos.com/calculator/ircjhtccbn
func BuyVolumeModifier(price float64) float64 {
	targetPrice := 7500.0 // we will get 1 at price 7500, and more below 7500
	flatness := 1000.0    // higher number buys more in the middle section. higher number gets more flat line, reduced to 0 at price 2000 * 10
	return math.Min(2, math.Exp(-(price-targetPrice)/flatness))
}

func SellVolumeModifier(price float64) float64 {
	// \exp\left(\frac{x-10000}{500}\right)
	targetPrice := 10500.0 // target to sell most x1 at 10000.0
	flatness := 500.0      // higher number sells more in the middle section, lower number sells fewer in the middle section.
	return math.Min(2, math.Exp((price-targetPrice)/flatness))
}

func VolumeByPriceChange(market types.Market, currentPrice float64, change float64, side types.SideType) float64 {
	volume := BaseVolumeByPriceChange(change)

	if side == types.SideTypeSell {
		volume *= SellVolumeModifier(currentPrice)
	} else {
		volume *= BuyVolumeModifier(currentPrice)
	}

	// at least the minimal quantity
	volume = math.Max(market.MinQuantity, volume)

	// modify volume for the min amount
	amount := currentPrice * volume
	if amount < market.MinAmount {
		ratio := market.MinAmount / amount
		volume *= ratio
	}

	volume = math.Trunc(volume*math.Pow10(market.VolumePrecision)) / math.Pow10(market.VolumePrecision)
	return volume
}

func BaseVolumeByPriceChange(change float64) float64 {
	return 0.2 * math.Exp((math.Abs(change)-3100.0)/1600.0)
	// 0.116*math.Exp(math.Abs(change)/2400) - 0.1
}
