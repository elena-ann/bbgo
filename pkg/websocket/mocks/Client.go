// Code generated by mockery v1.0.0. DO NOT EDIT.

package mocks

import context "context"
import mock "github.com/stretchr/testify/mock"
import time "time"
import websocket "github.maicoin.site/maicoin/garage/pkg/net/http/websocket"

// Client is an autogenerated mock type for the Client type
type Client struct {
	mock.Mock
}

// Close provides a mock function with given fields:
func (_m *Client) Close() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Connect provides a mock function with given fields: _a0
func (_m *Client) Connect(_a0 context.Context) error {
	ret := _m.Called(_a0)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context) error); ok {
		r0 = rf(_a0)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// IsConnected provides a mock function with given fields:
func (_m *Client) IsConnected() bool {
	ret := _m.Called()

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// Messages provides a mock function with given fields:
func (_m *Client) Messages() <-chan websocket.Message {
	ret := _m.Called()

	var r0 <-chan websocket.Message
	if rf, ok := ret.Get(0).(func() <-chan websocket.Message); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(<-chan websocket.Message)
		}
	}

	return r0
}

// OnConnect provides a mock function with given fields: _a0
func (_m *Client) OnConnect(_a0 func(websocket.Client)) {
	_m.Called(_a0)
}

// OnDisconnect provides a mock function with given fields: _a0
func (_m *Client) OnDisconnect(_a0 func(websocket.Client)) {
	_m.Called(_a0)
}

// Reconnect provides a mock function with given fields:
func (_m *Client) Reconnect() {
	_m.Called()
}

// SetReadTimeout provides a mock function with given fields: _a0
func (_m *Client) SetReadTimeout(_a0 time.Duration) {
	_m.Called(_a0)
}

// SetWriteTimeout provides a mock function with given fields: _a0
func (_m *Client) SetWriteTimeout(_a0 time.Duration) {
	_m.Called(_a0)
}

// WriteJSON provides a mock function with given fields: _a0
func (_m *Client) WriteJSON(_a0 interface{}) error {
	ret := _m.Called(_a0)

	var r0 error
	if rf, ok := ret.Get(0).(func(interface{}) error); ok {
		r0 = rf(_a0)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}
