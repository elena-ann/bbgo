package types

import (
	"context"
	"time"
)

type Exchange interface {
	PlatformFeeCurrency() string

	NewStream() Stream

	QueryAccount(ctx context.Context) (*Account, error)

	QueryAccountBalances(ctx context.Context) (BalanceMap, error)

	QueryKLines(ctx context.Context, symbol string, interval string, options KLineQueryOptions) ([]KLine, error)

	QueryTrades(ctx context.Context, symbol string, options *TradeQueryOptions) ([]Trade, error)
	BatchQueryTrades(ctx context.Context, symbol string, options *TradeQueryOptions) ([]Trade, error)

	SubmitOrder(ctx context.Context, order *SubmitOrder) error
}

type TradeQueryOptions struct {
	StartTime   *time.Time
	EndTime     *time.Time
	Limit       int64
	LastTradeID int64
}
