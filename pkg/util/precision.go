package util

import (
	"errors"
	"fmt"

	"github.maicoin.site/maicoin/garage/pkg/log"
)

var (
	ErrPrecisionLoss = errors.New("Precision loss when adjusting number")
)

const PositionPrecision = 8

func AdjustToPositionPrecision(volume int64, precision int32) int64 {
	adjustedVolume := AdjustToPrecision(volume, precision, PositionPrecision)
	return adjustedVolume
}

func AdjustToPrecision(fraction int64, precision, specifiedPrecision int32) int64 {
	fraction, err := AdjustToPrecisionOrError(fraction, precision, specifiedPrecision)
	if err != nil {
		log.Error(err)
	}
	return fraction
}

func AdjustToPrecisionOrError(fraction int64, precision, specifiedPrecision int32) (int64, error) {
	if specifiedPrecision == precision {
		return fraction, nil
	}
	if precision < specifiedPrecision {
		return fraction * Pow10(int64(specifiedPrecision)-int64(precision)), nil
	}

	originalFraction := fraction
	fraction = fraction / Pow10(int64(precision)-int64(specifiedPrecision))

	return fraction, fmt.Errorf("%v. original_fraction:%v, original_precision:%v, specified_precision:%v, adjusted_fraction:%v",
		ErrPrecisionLoss,
		originalFraction,
		precision,
		specifiedPrecision,
		fraction)
}
