// Code generated by MockGen. DO NOT EDIT.
// Source: acceptor/acceptor.go

// Package mock_acceptor is a generated GoMock package.
package mocks

import (
	net "net"
	reflect "reflect"
	time "time"

	gomock "github.com/golang/mock/gomock"
	acceptor "github.com/hnlxhzw/pitaya/acceptor"
)

// MockPlayerConn is a mock of PlayerConn interface
type MockPlayerConn struct {
	ctrl     *gomock.Controller
	recorder *MockPlayerConnMockRecorder
}

// MockPlayerConnMockRecorder is the mock recorder for MockPlayerConn
type MockPlayerConnMockRecorder struct {
	mock *MockPlayerConn
}

// NewMockPlayerConn creates a new mock instance
func NewMockPlayerConn(ctrl *gomock.Controller) *MockPlayerConn {
	mock := &MockPlayerConn{ctrl: ctrl}
	mock.recorder = &MockPlayerConnMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockPlayerConn) EXPECT() *MockPlayerConnMockRecorder {
	return m.recorder
}

// GetNextMessage mocks base method
func (m *MockPlayerConn) GetNextMessage() ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetNextMessage")
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetNextMessage indicates an expected call of GetNextMessage
func (mr *MockPlayerConnMockRecorder) GetNextMessage() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetNextMessage", reflect.TypeOf((*MockPlayerConn)(nil).GetNextMessage))
}

// Read mocks base method
func (m *MockPlayerConn) Read(b []byte) (int, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Read", b)
	ret0, _ := ret[0].(int)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Read indicates an expected call of Read
func (mr *MockPlayerConnMockRecorder) Read(b interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Read", reflect.TypeOf((*MockPlayerConn)(nil).Read), b)
}

// Write mocks base method
func (m *MockPlayerConn) Write(b []byte) (int, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Write", b)
	ret0, _ := ret[0].(int)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Write indicates an expected call of Write
func (mr *MockPlayerConnMockRecorder) Write(b interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Write", reflect.TypeOf((*MockPlayerConn)(nil).Write), b)
}

// Close mocks base method
func (m *MockPlayerConn) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close
func (mr *MockPlayerConnMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockPlayerConn)(nil).Close))
}

// LocalAddr mocks base method
func (m *MockPlayerConn) LocalAddr() net.Addr {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "LocalAddr")
	ret0, _ := ret[0].(net.Addr)
	return ret0
}

// LocalAddr indicates an expected call of LocalAddr
func (mr *MockPlayerConnMockRecorder) LocalAddr() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "LocalAddr", reflect.TypeOf((*MockPlayerConn)(nil).LocalAddr))
}

// RemoteAddr mocks base method
func (m *MockPlayerConn) RemoteAddr() net.Addr {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RemoteAddr")
	ret0, _ := ret[0].(net.Addr)
	return ret0
}

// RemoteAddr indicates an expected call of RemoteAddr
func (mr *MockPlayerConnMockRecorder) RemoteAddr() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RemoteAddr", reflect.TypeOf((*MockPlayerConn)(nil).RemoteAddr))
}

// SetDeadline mocks base method
func (m *MockPlayerConn) SetDeadline(t time.Time) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetDeadline", t)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetDeadline indicates an expected call of SetDeadline
func (mr *MockPlayerConnMockRecorder) SetDeadline(t interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetDeadline", reflect.TypeOf((*MockPlayerConn)(nil).SetDeadline), t)
}

// SetReadDeadline mocks base method
func (m *MockPlayerConn) SetReadDeadline(t time.Time) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetReadDeadline", t)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetReadDeadline indicates an expected call of SetReadDeadline
func (mr *MockPlayerConnMockRecorder) SetReadDeadline(t interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetReadDeadline", reflect.TypeOf((*MockPlayerConn)(nil).SetReadDeadline), t)
}

// SetWriteDeadline mocks base method
func (m *MockPlayerConn) SetWriteDeadline(t time.Time) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetWriteDeadline", t)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetWriteDeadline indicates an expected call of SetWriteDeadline
func (mr *MockPlayerConnMockRecorder) SetWriteDeadline(t interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetWriteDeadline", reflect.TypeOf((*MockPlayerConn)(nil).SetWriteDeadline), t)
}

// MockAcceptor is a mock of Acceptor interface
type MockAcceptor struct {
	ctrl     *gomock.Controller
	recorder *MockAcceptorMockRecorder
}

// MockAcceptorMockRecorder is the mock recorder for MockAcceptor
type MockAcceptorMockRecorder struct {
	mock *MockAcceptor
}

// NewMockAcceptor creates a new mock instance
func NewMockAcceptor(ctrl *gomock.Controller) *MockAcceptor {
	mock := &MockAcceptor{ctrl: ctrl}
	mock.recorder = &MockAcceptorMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockAcceptor) EXPECT() *MockAcceptorMockRecorder {
	return m.recorder
}

// ListenAndServe mocks base method
func (m *MockAcceptor) ListenAndServe() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "ListenAndServe")
}

// ListenAndServe indicates an expected call of ListenAndServe
func (mr *MockAcceptorMockRecorder) ListenAndServe() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ListenAndServe", reflect.TypeOf((*MockAcceptor)(nil).ListenAndServe))
}

// Stop mocks base method
func (m *MockAcceptor) Stop() {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Stop")
}

// Stop indicates an expected call of Stop
func (mr *MockAcceptorMockRecorder) Stop() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Stop", reflect.TypeOf((*MockAcceptor)(nil).Stop))
}

// GetAddr mocks base method
func (m *MockAcceptor) GetAddr() string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetAddr")
	ret0, _ := ret[0].(string)
	return ret0
}

// GetAddr indicates an expected call of GetAddr
func (mr *MockAcceptorMockRecorder) GetAddr() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetAddr", reflect.TypeOf((*MockAcceptor)(nil).GetAddr))
}

// GetConnChan mocks base method
func (m *MockAcceptor) GetConnChan() chan acceptor.PlayerConn {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetConnChan")
	ret0, _ := ret[0].(chan acceptor.PlayerConn)
	return ret0
}

// GetConnChan indicates an expected call of GetConnChan
func (mr *MockAcceptorMockRecorder) GetConnChan() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetConnChan", reflect.TypeOf((*MockAcceptor)(nil).GetConnChan))
}
