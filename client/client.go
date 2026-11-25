// Copyright (c) TFG Co. All Rights Reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package client

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hnlxhzw/pitaya/acceptor"

	"github.com/gorilla/websocket"
	"github.com/hnlxhzw/pitaya"
	"github.com/hnlxhzw/pitaya/conn/codec"
	"github.com/hnlxhzw/pitaya/conn/message"
	"github.com/hnlxhzw/pitaya/conn/packet"
	"github.com/hnlxhzw/pitaya/logger"
	"github.com/hnlxhzw/pitaya/session"
	"github.com/hnlxhzw/pitaya/util/compression"
	"github.com/sirupsen/logrus"
)

// HandshakeSys struct
type HandshakeSys struct {
	Dict       map[string]uint16 `json:"dict"`
	Heartbeat  int               `json:"heartbeat"`
	Serializer string            `json:"serializer"`
}

// HandshakeData struct
type HandshakeData struct {
	Code int          `json:"code"`
	Sys  HandshakeSys `json:"sys"`
}

type pendingRequest struct {
	msg    *message.Message
	sentAt time.Time
}

// Client struct
type Client struct {
	// EchoTraceAccount identifies a diagnostic bot; zero disables SDK wire logging.
	EchoTraceAccount    atomic.Int64
	conn                net.Conn
	Connected           bool // Use ConnectedStatus for concurrent reads.
	stateMu             sync.RWMutex
	dialMu              sync.Mutex
	workers             sync.WaitGroup
	packetEncoder       codec.PacketEncoder
	packetDecoder       codec.PacketDecoder
	packetChan          chan *packet.Packet
	IncomingMsgChan     chan *message.Message
	pendingChan         chan bool
	pendingRequests     map[uint]*pendingRequest
	pendingReqMutex     sync.Mutex
	requestTimeout      time.Duration
	closeChan           chan struct{}
	nextID              uint32
	messageEncoder      message.Encoder
	clientHandshakeData *session.HandshakeData
	writeRW             sync.RWMutex
}

// MsgChannel return the incoming message channel
func (c *Client) MsgChannel() chan *message.Message {
	return c.IncomingMsgChan
}

// ConnectedStatus return the connection status
func (c *Client) ConnectedStatus() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.Connected
}

// Done is closed when this established connection disconnects.
func (c *Client) Done() <-chan struct{} {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.closeChan
}

// New returns a new client
func New(logLevel logrus.Level, requestTimeout ...time.Duration) *Client {
	//l := logrus.New()
	//l.Formatter = &logrus.TextFormatter{}
	//l.SetLevel(logLevel)
	//
	//logger.Log = l

	reqTimeout := 5 * time.Second
	if len(requestTimeout) > 0 {
		reqTimeout = requestTimeout[0]
	}

	return &Client{
		Connected:       false,
		packetEncoder:   codec.NewPomeloPacketEncoder(),
		packetDecoder:   codec.NewPomeloPacketDecoder(),
		packetChan:      make(chan *packet.Packet, 10),
		pendingRequests: make(map[uint]*pendingRequest),
		requestTimeout:  reqTimeout,
		// 30 here is the limit of inflight messages
		// TODO this should probably be configurable
		pendingChan:    make(chan bool, 30),
		messageEncoder: message.NewMessagesEncoder(false),
		clientHandshakeData: &session.HandshakeData{
			Sys: session.HandshakeClientData{
				Platform:    "mac",
				LibVersion:  "0.3.5-release",
				BuildNumber: "20",
				Version:     "2.1",
			},
			User: map[string]interface{}{
				"age": 30,
			},
		},
	}
}

// SetClientHandshakeData sets the data to send inside handshake
func (c *Client) SetClientHandshakeData(data *session.HandshakeData) {
	c.clientHandshakeData = data
}

func (c *Client) sendHandshakeRequest() error {
	enc, err := json.Marshal(c.clientHandshakeData)
	if err != nil {
		return err
	}

	p, err := c.packetEncoder.Encode(packet.Handshake, enc)
	if err != nil {
		return err
	}

	_, err = c.write(p)
	return err
}

func (c *Client) handleHandshakeResponse() error {
	buf := bytes.NewBuffer(nil)
	packets, err := c.readPackets(buf)
	if err != nil {
		return err
	}

	if len(packets) == 0 {
		return io.ErrUnexpectedEOF
	}
	handshakePacket := packets[0]
	if handshakePacket.Type != packet.Handshake {
		return fmt.Errorf("got first packet from server that is not a handshake, aborting")
	}

	handshake := &HandshakeData{}
	if compression.IsCompressed(handshakePacket.Data) {
		handshakePacket.Data, err = compression.InflateData(handshakePacket.Data)
		if err != nil {
			return err
		}
	}

	err = json.Unmarshal(handshakePacket.Data, handshake)
	if err != nil {
		return err
	}

	logger.Log.Debug("got handshake from sv, data: %v", handshake)

	if handshake.Sys.Dict != nil {
		message.SetDictionary(handshake.Sys.Dict)
	}
	p, err := c.packetEncoder.Encode(packet.HandshakeAck, []byte{})
	if err != nil {
		return err
	}
	_, err = c.write(p)
	if err != nil {
		return err
	}

	if handshake.Sys.Heartbeat <= 0 {
		return fmt.Errorf("invalid handshake heartbeat interval")
	}
	c.stateMu.Lock()
	c.closeChan = make(chan struct{})
	c.Connected = true
	c.stateMu.Unlock()

	for _, run := range []func(){func() { c.sendHeartbeats(handshake.Sys.Heartbeat) }, c.handleServerMessages, c.handlePackets, c.pendingRequestsReaper} {
		c.workers.Add(1)
		go func(run func()) { defer c.workers.Done(); run() }(run)
	}

	return nil
}

// pendingRequestsReaper delete timedout requests
func (c *Client) pendingRequestsReaper() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			toDelete := make([]*pendingRequest, 0)
			c.pendingReqMutex.Lock()
			for _, v := range c.pendingRequests {
				if time.Now().Sub(v.sentAt) > c.requestTimeout {
					toDelete = append(toDelete, v)
				}
			}
			for _, r := range toDelete {
				delete(c.pendingRequests, r.msg.ID)
				<-c.pendingChan
			}
			c.pendingReqMutex.Unlock()
			for _, pendingReq := range toDelete {
				if acc := c.EchoTraceAccount.Load(); acc != 0 {
					fmt.Printf("echo_diag stage=bot_sdk_timeout account=%d mid=%d age_us=%d at_us=%d\n", acc, pendingReq.msg.ID, time.Since(pendingReq.sentAt).Microseconds(), time.Now().UnixMicro())
				}
				err := pitaya.Error(errors.New("request timeout"), "PIT-504")
				errMarshalled, _ := json.Marshal(err)
				// send a timeout to incoming msg chan
				m := &message.Message{
					Type:  message.Response,
					ID:    pendingReq.msg.ID,
					Route: pendingReq.msg.Route,
					Data:  errMarshalled,
					Err:   true,
				}

				select {
				case c.IncomingMsgChan <- m:
				case <-c.closeChan:
					return
				}
			}

		case <-c.closeChan:
			return
		}
	}
}

func (c *Client) handlePackets() {
	for {
		select {
		case p := <-c.packetChan:
			switch p.Type {
			case packet.Data:
				//handle data
				logger.Log.Debug("got data: %s", string(p.Data))
				m, err := message.Decode(p.Data)
				if err != nil {
					logger.Log.Errorf("error decoding msg from sv: %v", err)
					continue
				}
				if m.Type == message.Response && c.EchoTraceAccount.Load() != 0 {
					fmt.Printf("echo_diag stage=bot_wire account=%d mid=%d error_frame=%t at_us=%d\n", c.EchoTraceAccount.Load(), m.ID, m.Err, time.Now().UnixMicro())
				}
				if m.Type == message.Response {
					c.pendingReqMutex.Lock()
					if _, ok := c.pendingRequests[m.ID]; ok {
						delete(c.pendingRequests, m.ID)
						<-c.pendingChan
					} else {
						c.pendingReqMutex.Unlock()
						continue // do not process msg for already timedout request
					}
					c.pendingReqMutex.Unlock()
				}
				select {
				case c.IncomingMsgChan <- m:
				case <-c.closeChan:
					return
				}
			case packet.Kick:
				logger.Log.Warn("got kick packet from the server! disconnecting...")
				c.Disconnect()
			}
		case <-c.closeChan:
			return
		}
	}
}

func (c *Client) readPackets(buf *bytes.Buffer) ([]*packet.Packet, error) {
	data := make([]byte, 2048)
	_, framed := c.conn.(*acceptor.WSConn)
	for {
		n, readErr := c.conn.Read(data)
		if n > 0 {
			buf.Write(data[:n])
		}
		if readErr != nil && readErr != io.EOF {
			return nil, readErr
		}
		// A WebSocket reader returns EOF at the end of an individual frame.
		if framed && readErr == nil {
			continue
		}
		if buf.Len() >= codec.HeadLength {
			size, _, err := codec.ParseHeader(buf.Bytes()[:codec.HeadLength])
			if err != nil {
				return nil, err
			}
			if buf.Len()-codec.HeadLength >= size {
				packets, err := c.packetDecoder.Decode(buf.Bytes())
				if err != nil {
					return nil, err
				}
				for _, p := range packets {
					buf.Next(codec.HeadLength + p.Length)
				}
				return packets, nil
			}
		}
		if readErr == io.EOF && !framed {
			if buf.Len() > 0 {
				return nil, io.ErrUnexpectedEOF
			}
			return nil, io.EOF
		}
	}

}

func (c *Client) handleServerMessages() {
	buf := bytes.NewBuffer(nil)
	defer c.Disconnect()
	for c.ConnectedStatus() {
		packets, err := c.readPackets(buf)
		if err != nil {
			if c.ConnectedStatus() {
				logger.Log.Error(err)
			}
			break
		}

		for _, p := range packets {
			select {
			case c.packetChan <- p:
			case <-c.closeChan:
				return
			}
		}
	}
}

func (c *Client) sendHeartbeats(interval int) {
	t := time.NewTicker(time.Duration(interval) * time.Second)
	defer func() {
		t.Stop()
		c.Disconnect()
	}()
	for {
		select {
		case <-t.C:
			p, _ := c.packetEncoder.Encode(packet.Heartbeat, []byte{})
			_, err := c.write(p)
			if err != nil {
				logger.Log.Errorf("error sending heartbeat to server: %s", err.Error())
				return
			}
		case <-c.closeChan:
			return
		}
	}
}

// Disconnect disconnects the client
func (c *Client) Disconnect() {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.Connected {
		c.Connected = false
		close(c.closeChan)
		c.conn.Close()
	}
}

// Called under dialMu, before starting a new generation of workers.
func (c *Client) prepareConnection() error {
	if c.ConnectedStatus() {
		return fmt.Errorf("client already connected")
	}
	c.workers.Wait()
	c.pendingReqMutex.Lock()
	c.pendingRequests = make(map[uint]*pendingRequest)
	c.pendingChan = make(chan bool, cap(c.pendingChan))
	c.pendingReqMutex.Unlock()
	c.packetChan = make(chan *packet.Packet, cap(c.packetChan))
	return nil
}

// ConnectTo connects to the server at addr, for now the only supported protocol is tcp
// if tlsConfig is sent, it connects using TLS
func (c *Client) ConnectTo(addr string, tlsConfig ...*tls.Config) error {
	c.dialMu.Lock()
	defer c.dialMu.Unlock()
	if err := c.prepareConnection(); err != nil {
		return err
	}

	var conn net.Conn
	var err error
	if len(tlsConfig) > 0 {
		conn, err = tls.Dial("tcp", addr, tlsConfig[0])
	} else {
		conn, err = net.Dial("tcp", addr)
	}
	if err != nil {
		return err
	}
	c.conn = conn
	c.IncomingMsgChan = make(chan *message.Message, 10)

	if err = c.handleHandshake(); err != nil {
		c.conn.Close()
		return err
	}

	return nil
}

// ConnectToWS connects using webshocket protocol
func (c *Client) ConnectToWS(addr string, path string, tlsConfig ...*tls.Config) error {
	c.dialMu.Lock()
	defer c.dialMu.Unlock()
	if err := c.prepareConnection(); err != nil {
		return err
	}

	u := url.URL{Scheme: "ws", Host: addr, Path: path}
	dialer := *websocket.DefaultDialer
	dialer.WriteBufferSize = 40960
	dialer.ReadBufferSize = 40960

	if len(tlsConfig) > 0 {
		dialer.TLSClientConfig = tlsConfig[0]
		u.Scheme = "wss"
	}

	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}

	c.conn, err = acceptor.NewWSConn(conn)
	if err != nil {
		conn.Close()
		return err
	}

	c.IncomingMsgChan = make(chan *message.Message, 10)

	if err = c.handleHandshake(); err != nil {
		c.conn.Close()
		return err
	}

	return nil
}

func (c *Client) handleHandshake() error {
	if err := c.sendHandshakeRequest(); err != nil {
		return err
	}

	if err := c.handleHandshakeResponse(); err != nil {
		return err
	}
	return nil
}

// SendRequest sends a request to the server
func (c *Client) SendRequest(route string, data []byte) (uint, error) {
	return c.sendMsg(message.Request, route, data)
}

// SendNotify sends a notify to the server
func (c *Client) SendNotify(route string, data []byte) error {
	_, err := c.sendMsg(message.Notify, route, data)
	return err
}

func (c *Client) buildPacket(msg message.Message) ([]byte, error) {
	encMsg, err := c.messageEncoder.Encode(&msg)
	if err != nil {
		return nil, err
	}
	p, err := c.packetEncoder.Encode(packet.Data, encMsg)
	if err != nil {
		return nil, err
	}

	return p, nil
}

// sendMsg sends the request to the server
func (c *Client) sendMsg(msgType message.Type, route string, data []byte) (uint, error) {
	// TODO mount msg and encode
	c.dialMu.Lock()
	defer c.dialMu.Unlock()
	if c.conn == nil {
		return 0, io.ErrClosedPipe
	}
	select {
	case <-c.closeChan:
		return 0, io.ErrClosedPipe
	default:
	}
	m := message.Message{
		Type:  msgType,
		ID:    uint(atomic.AddUint32(&c.nextID, 1)),
		Route: route,
		Data:  data,
		Err:   false,
	}
	p, err := c.buildPacket(m)
	if err != nil {
		return m.ID, err
	}
	if msgType == message.Request {
		select {
		case c.pendingChan <- true:
		case <-c.closeChan:
			return m.ID, io.ErrClosedPipe
		}
		c.pendingReqMutex.Lock()
		if _, ok := c.pendingRequests[m.ID]; !ok {
			c.pendingRequests[m.ID] = &pendingRequest{
				msg:    &m,
				sentAt: time.Now(),
			}
		}
		c.pendingReqMutex.Unlock()
	}

	_, err = c.write(p)
	if err != nil && msgType == message.Request {
		c.pendingReqMutex.Lock()
		if _, ok := c.pendingRequests[m.ID]; ok {
			delete(c.pendingRequests, m.ID)
			<-c.pendingChan
		}
		c.pendingReqMutex.Unlock()
	}
	return m.ID, err
}

func (c *Client) write(p []byte) (n int, err error) {
	defer c.writeRW.Unlock()
	c.writeRW.Lock()
	return c.conn.Write(p)
}
