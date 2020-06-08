package util

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestAdjustPrecision(t *testing.T) {
	volume := AdjustToPositionPrecision(int64(12345), int32(4))
	assert.Equal(t, volume, int64(123450000))
}

func TestAdjustToPrecisionPositve(t *testing.T) {
	oriFraction := int64(1234)
	oriPrecision := int32(4)
	ansFraction := int64(12340000)
	ansPrecision := int32(8)

	resFraction := AdjustToPrecision(oriFraction, oriPrecision, ansPrecision)
	assert.Equal(t, ansFraction, resFraction)
}
func TestAdjustToPrecisionNegative(t *testing.T) {
	oriFraction := int64(-1234)
	oriPrecision := int32(4)
	ansFraction := int64(-12340000)
	ansPrecision := int32(8)

	resFraction := AdjustToPrecision(oriFraction, oriPrecision, ansPrecision)
	assert.Equal(t, ansFraction, resFraction)
}
