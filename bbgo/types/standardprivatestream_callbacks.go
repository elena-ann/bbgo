// Code generated by "callbackgen -type StandardPrivateStream -interface"; DO NOT EDIT.

package types

import ()

func (stream *StandardPrivateStream) OnTrade(cb func(trade *Trade)) {
	stream.tradeCallbacks = append(stream.tradeCallbacks, cb)
}

func (stream *StandardPrivateStream) EmitTrade(trade *Trade) {
	for _, cb := range stream.tradeCallbacks {
		cb(trade)
	}
}

func (stream *StandardPrivateStream) OnBalanceSnapshot(cb func(balanceSnapshot map[string]Balance)) {
	stream.balanceSnapshotCallbacks = append(stream.balanceSnapshotCallbacks, cb)
}

func (stream *StandardPrivateStream) EmitBalanceSnapshot(balanceSnapshot map[string]Balance) {
	for _, cb := range stream.balanceSnapshotCallbacks {
		cb(balanceSnapshot)
	}
}

func (stream *StandardPrivateStream) OnKLineClosed(cb func(kline KLine)) {
	stream.kLineClosedCallbacks = append(stream.kLineClosedCallbacks, cb)
}

func (stream *StandardPrivateStream) EmitKLineClosed(kline KLine) {
	for _, cb := range stream.kLineClosedCallbacks {
		cb(kline)
	}
}

type StandardPrivateStreamEventHub interface {
	OnTrade(cb func(trade *Trade))

	OnBalanceSnapshot(cb func(balanceSnapshot map[string]Balance))

	OnKLineClosed(cb func(kline KLine))
}