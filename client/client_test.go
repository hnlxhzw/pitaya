package client

import (
	"bytes"
	"github.com/gorilla/websocket"
	"github.com/hnlxhzw/pitaya/conn/codec"
	"github.com/hnlxhzw/pitaya/conn/packet"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/hnlxhzw/pitaya/conn/message"
	"github.com/hnlxhzw/pitaya/helpers"
	"github.com/hnlxhzw/pitaya/mocks"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestSendRequestShouldTimeout(t *testing.T) {
	c := New(logrus.InfoLevel, 100*time.Millisecond)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockConn := mocks.NewMockPlayerConn(ctrl)
	c.conn = mockConn
	c.closeChan = make(chan struct{})
	defer close(c.closeChan)
	go c.pendingRequestsReaper()

	route := "com.sometest.route"
	data := []byte{0x02, 0x03, 0x04}

	m := message.Message{
		Type:  message.Request,
		ID:    1,
		Route: route,
		Data:  data,
		Err:   false,
	}

	pkt, err := c.buildPacket(m)
	assert.NoError(t, err)

	mockConn.EXPECT().Write(pkt)

	c.IncomingMsgChan = make(chan *message.Message, 10)

	c.nextID = 0
	c.SendRequest(route, data)

	msg := helpers.ShouldEventuallyReceive(t, c.IncomingMsgChan, 2*time.Second).(*message.Message)

	assert.Equal(t, true, msg.Err)
}

func TestDisconnectUnblocksConcurrentRequests(t *testing.T) {
	c := New(logrus.WarnLevel)
	conn, peer := net.Pipe()
	defer peer.Close()
	c.conn = conn
	c.Connected = true
	c.closeChan = make(chan struct{})
	c.pendingChan = make(chan bool, 1)
	c.pendingChan <- true
	done := make(chan error, 1)
	go func() { _, err := c.SendRequest("test.echo", nil); done <- err }()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c.Disconnect() }()
	}
	wg.Wait()
	select {
	case err := <-done:
		if err != io.ErrClosedPipe {
			t.Fatalf("send returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("send stuck after disconnect")
	}
	select {
	case <-c.Done():
	default:
		t.Fatal("missing disconnect signal")
	}
}
func TestWriteFailureReleasesPendingSlot(t *testing.T) {
	c := New(logrus.WarnLevel)
	conn, peer := net.Pipe()
	c.conn = conn
	defer conn.Close()
	peer.Close()
	_, err := c.SendRequest("test.echo", nil)
	if err == nil || len(c.pendingRequests) != 0 || len(c.pendingChan) != 0 {
		t.Fatalf("write err=%v pending=%d slots=%d", err, len(c.pendingRequests), len(c.pendingChan))
	}
	if _, err := c.readPackets(&bytes.Buffer{}); err != io.EOF {
		t.Fatalf("closed TCP read: %v", err)
	}
}

func TestWebSocketPeerCloseAndReuse(t *testing.T) {
	up := websocket.Upgrader{}
	handshake, err := codec.NewPomeloPacketEncoder().Encode(packet.Handshake, []byte(`{"code":200,"sys":{"heartbeat":30}}`))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, e := up.Upgrade(w, r, nil)
		if e != nil {
			t.Error(e)
			return
		}
		defer ws.Close()
		if _, _, e = ws.ReadMessage(); e != nil {
			t.Error(e)
			return
		}
		if e = ws.WriteMessage(websocket.BinaryMessage, handshake); e != nil {
			t.Error(e)
			return
		}
		if _, _, e = ws.ReadMessage(); e != nil {
			t.Error(e)
		}
	}))
	defer server.Close()
	c := New(logrus.WarnLevel)
	for i := 0; i < 4; i++ {
		if e := c.ConnectToWS(strings.TrimPrefix(server.URL, "http://"), "/"); e != nil {
			t.Fatal(e)
		}
		select {
		case <-c.Done():
		case <-time.After(2 * time.Second):
			t.Fatal("peer close not propagated")
		}
		if c.ConnectedStatus() {
			t.Fatal("client still connected")
		}
	}
	c.workers.Wait()
}
