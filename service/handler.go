// Copyright (c) nano Author and TFG Co. All Rights Reserved.
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

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/hnlxhzw/pitaya/util"
	"strings"
	"sync"
	"time"

	"github.com/hnlxhzw/pitaya/acceptor"

	"github.com/google/uuid"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/hnlxhzw/pitaya/agent"
	"github.com/hnlxhzw/pitaya/cluster"
	"github.com/hnlxhzw/pitaya/component"
	"github.com/hnlxhzw/pitaya/conn/codec"
	"github.com/hnlxhzw/pitaya/conn/message"
	"github.com/hnlxhzw/pitaya/conn/packet"
	"github.com/hnlxhzw/pitaya/constants"
	pcontext "github.com/hnlxhzw/pitaya/context"
	"github.com/hnlxhzw/pitaya/docgenerator"
	e "github.com/hnlxhzw/pitaya/errors"
	"github.com/hnlxhzw/pitaya/logger"
	"github.com/hnlxhzw/pitaya/metrics"
	"github.com/hnlxhzw/pitaya/protos"
	"github.com/hnlxhzw/pitaya/route"
	"github.com/hnlxhzw/pitaya/serialize"
	"github.com/hnlxhzw/pitaya/session"
	"github.com/hnlxhzw/pitaya/timer"
	"github.com/hnlxhzw/pitaya/tracing"
)

var (
	handlers    = make(map[string]*component.Handler) // all handler method
	handlerType = "handler"
)

type (
	// HandlerService service
	HandlerService struct {
		appDieChan chan bool // die channel app
		//chLocalProcess     chan unhandledMessage // channel of messages that will be processed locally
		//chRemoteProcess    chan unhandledMessage // channel of messages that will be processed remotely
		chLocalProcessMap       map[int]chan unhandledMessage // channel map of messages that will be processed locally 改成多个channel
		chRemoteProcessMap      map[int]chan unhandledMessage // channel map of messages that will be processed remotely
		localProcessBufferSize  int
		remoteProcessBufferSize int
		decoder                 codec.PacketDecoder // binary decoder
		encoder                 codec.PacketEncoder // binary encoder
		heartbeatTimeout        time.Duration
		messagesBufferSize      int
		remoteService           *RemoteService
		serializer              serialize.Serializer          // message serializer
		server                  *cluster.Server               // server obj
		services                map[string]*component.Service // all registered service
		messageEncoder          message.Encoder
		metricsReporters        []metrics.Reporter
		rw                      sync.RWMutex
		dispatchNum             int
	}

	unhandledMessage struct {
		ctx   context.Context
		agent *agent.Agent
		route *route.Route
		msg   *message.Message
	}
)

// NewHandlerService creates and returns a new handler service
func NewHandlerService(
	dieChan chan bool,
	packetDecoder codec.PacketDecoder,
	packetEncoder codec.PacketEncoder,
	serializer serialize.Serializer,
	heartbeatTime time.Duration,
	messagesBufferSize,
	localProcessBufferSize,
	remoteProcessBufferSize int,
	server *cluster.Server,
	remoteService *RemoteService,
	messageEncoder message.Encoder,
	metricsReporters []metrics.Reporter,
	dispatchNum int,
) *HandlerService {

	h := &HandlerService{
		services: make(map[string]*component.Service),
		//chLocalProcess:     make(chan unhandledMessage, localProcessBufferSize),
		//chRemoteProcess:    make(chan unhandledMessage, remoteProcessBufferSize),
		//因为原来的有n个dispatch对应
		// 一个channel读取数据 这样有可能会出现请求的先后顺序问题
		//改成把每个dispatch进程都对应一个channel 每一个conn 对应的数据都会发到同一个channel中
		chLocalProcessMap:       make(map[int]chan unhandledMessage, dispatchNum),
		chRemoteProcessMap:      make(map[int]chan unhandledMessage, dispatchNum),
		localProcessBufferSize:  localProcessBufferSize,
		remoteProcessBufferSize: remoteProcessBufferSize,
		decoder:                 packetDecoder,
		encoder:                 packetEncoder,
		messagesBufferSize:      messagesBufferSize,
		serializer:              serializer,
		heartbeatTimeout:        heartbeatTime,
		appDieChan:              dieChan,
		server:                  server,
		remoteService:           remoteService,
		messageEncoder:          messageEncoder,
		metricsReporters:        metricsReporters,
		dispatchNum:             dispatchNum,
	}

	return h
}

// Dispatch message to corresponding logic handler
func (h *HandlerService) Dispatch(thread int, wg *sync.WaitGroup) {
	h.rw.Lock()

	defer util.AutoRecover("Dispatch")
	// TODO: This timer is being stopped multiple times, it probably doesn't need to be stopped here
	defer timer.GlobalTicker.Stop()

	defer func() {
		h.rw.Lock()
		close(h.chLocalProcessMap[thread])
		delete(h.chLocalProcessMap, thread)
		close(h.chRemoteProcessMap[thread])
		delete(h.chRemoteProcessMap, thread)
		h.rw.Unlock()
	}()

	chLocalProcess := make(chan unhandledMessage, h.localProcessBufferSize)
	h.chLocalProcessMap[thread] = chLocalProcess
	chRemoteProcess := make(chan unhandledMessage, h.remoteProcessBufferSize)
	h.chRemoteProcessMap[thread] = chRemoteProcess

	h.rw.Unlock()
	wg.Done()

	for {
		// Calls to remote servers block calls to local server
		select {
		case lm := <-chLocalProcess:
			metrics.ReportMessageProcessDelayFromCtx(lm.ctx, h.metricsReporters, "local")
			h.localProcess(lm.ctx, lm.agent, lm.route, lm.msg)

		case rm := <-chRemoteProcess:
			metrics.ReportMessageProcessDelayFromCtx(rm.ctx, h.metricsReporters, "remote")
			h.remoteService.remoteProcess(rm.ctx, nil, rm.agent, rm.route, rm.msg)

		case <-timer.GlobalTicker.C: // execute cron task
			timer.Cron()

		case t := <-timer.Manager.ChCreatedTimer: // new Timers
			timer.AddTimer(t)

		case id := <-timer.Manager.ChClosingTimer: // closing Timers
			timer.RemoveTimer(id)
		}
	}
}

// Register registers components
func (h *HandlerService) Register(comp component.Component, opts []component.Option) error {
	s := component.NewService(comp, opts)

	if _, ok := h.services[s.Name]; ok {
		return fmt.Errorf("handler: service already defined: %s", s.Name)
	}

	if err := s.ExtractHandler(); err != nil {
		return err
	}

	// register all handlers
	h.services[s.Name] = s
	for name, handler := range s.Handlers {
		handlers[fmt.Sprintf("%s.%s", s.Name, name)] = handler
	}
	return nil
}

// Handle handles messages from a conn
func (h *HandlerService) Handle(conn acceptor.PlayerConn) {
	defer util.AutoRecover("HandlerService Handle")
	// create a client agent and startup write goroutine
	a := agent.NewAgent(conn, h.decoder, h.encoder, h.serializer, h.heartbeatTimeout, h.messagesBufferSize, h.appDieChan, h.messageEncoder, h.metricsReporters)

	// startup agent goroutine
	go a.Handle()

	logger.Log.Debugf("New session established: %s", a.String())

	// guarantee agent related resource is destroyed
	defer func() {
		a.Session.Close()
		logger.Log.Debugf("Session read goroutine exit, SessionID=%d, UID=%s", a.Session.ID(), a.Session.UID())
	}()

	for {
		//change by shawn 状态为已关闭 就不需要再去读数据了
		if a.GetStatus() == constants.StatusClosed {
			logger.Log.Debugf("Agent Close!!!!")
			return
		}
		msg, err := conn.GetNextMessage()

		if err != nil {
			if err != constants.ErrConnectionClosed {
				logger.Log.Errorf("Error reading next available message: %s", err.Error())
			}

			return
		}

		packets, err := h.decoder.Decode(msg)
		if err != nil {
			logger.Log.Errorf("Failed to decode message: %s", err.Error())
			return
		}

		if len(packets) < 1 {
			logger.Log.Warnf("Read no packets, data: %v", msg)
			continue
		}

		// process all packet
		for i := range packets {
			if err := h.processPacket(a, packets[i]); err != nil {
				logger.Log.Errorf("Failed to process packet: %s", err.Error())
				return
			}
		}
	}
}

func (h *HandlerService) processPacket(a *agent.Agent, p *packet.Packet) error {
	switch p.Type {
	case packet.Handshake:
		logger.Log.Debug("Received handshake packet")
		if err := a.SendHandshakeResponse(); err != nil {
			logger.Log.Errorf("Error sending handshake response: %s", err.Error())
			return err
		}
		logger.Log.Debugf("Session handshake Id=%d, Remote=%s", a.Session.ID(), a.RemoteAddr())

		// Parse the json sent with the handshake by the client
		handshakeData := &session.HandshakeData{}
		err := json.Unmarshal(p.Data, handshakeData)
		if err != nil {
			a.SetStatus(constants.StatusClosed)
			return fmt.Errorf("Invalid handshake data. Id=%d", a.Session.ID())
		}

		a.Session.SetHandshakeData(handshakeData)
		a.SetStatus(constants.StatusHandshake)
		err = a.Session.Set(constants.IPVersionKey, a.IPVersion())
		if err != nil {
			logger.Log.Warnf("failed to save ip version on session: %q\n", err)
		}

		logger.Log.Debug("Successfully saved handshake data")

	case packet.HandshakeAck:
		a.SetStatus(constants.StatusWorking)
		logger.Log.Debugf("Receive handshake ACK Id=%d, Remote=%s", a.Session.ID(), a.RemoteAddr())

	case packet.Data:
		if a.GetStatus() < constants.StatusWorking {
			return fmt.Errorf("receive data on socket which is not yet ACK, session will be closed immediately, remote=%s",
				a.RemoteAddr().String())
		}

		msg, err := message.Decode(p.Data)
		if err != nil {
			return err
		}
		h.processMessage(a, msg)

	case packet.Heartbeat:
		// expected
	}

	a.SetLastAt()
	return nil
}

func (h *HandlerService) processMessage(a *agent.Agent, msg *message.Message) {
	requestID := uuid.New()
	ctx := pcontext.AddToPropagateCtx(context.Background(), constants.StartTimeKey, time.Now().UnixNano())
	ctx = pcontext.AddToPropagateCtx(ctx, constants.RouteKey, msg.Route)
	ctx = pcontext.AddToPropagateCtx(ctx, constants.RequestIDKey, requestID.String())
	tags := opentracing.Tags{
		"local.id":   h.server.ID,
		"span.kind":  "server",
		"msg.type":   strings.ToLower(msg.Type.String()),
		"user.id":    a.Session.UID(),
		"request.id": requestID.String(),
	}
	ctx = tracing.StartSpan(ctx, msg.Route, tags)
	ctx = context.WithValue(ctx, constants.SessionCtxKey, a.Session)

	r, err := route.Decode(msg.Route)
	if err != nil {
		logger.Log.Errorf("Failed to decode route: %s", err.Error())
		a.AnswerWithError(ctx, msg.ID, e.NewError(err, e.ErrBadRequestCode.Desc, e.ErrBadRequestCode.ErrorCode))
		return
	}

	if r.SvType == "" {
		r.SvType = h.server.Type
	}

	message := unhandledMessage{
		ctx:   ctx,
		agent: a,
		route: r,
		msg:   msg,
	}

	dispatchIndex := h.GetChProcessIndex(a)
	if r.SvType == h.server.Type {
		h.chLocalProcessMap[dispatchIndex] <- message
	} else {
		if h.remoteService != nil {
			h.chRemoteProcessMap[dispatchIndex] <- message
		} else {
			logger.Log.Warnf("request made to another server type but no remoteService running")
		}
	}
}

func (h *HandlerService) ProcessMessageForHttp(routeStr string, data []byte) (*protos.Response, error) {
	msg := message.New()
	msg.Type = message.Request
	msg.Route = routeStr
	msg.Data = data
	agentForHttp := agent.NewAgentForHttp()
	requestID := uuid.New()
	ctx := pcontext.AddToPropagateCtx(context.Background(), constants.StartTimeKey, time.Now().UnixNano())
	ctx = pcontext.AddToPropagateCtx(ctx, constants.RouteKey, msg.Route)
	ctx = pcontext.AddToPropagateCtx(ctx, constants.RequestIDKey, requestID.String())
	tags := opentracing.Tags{
		"local.id":   h.server.ID,
		"span.kind":  "server",
		"msg.type":   strings.ToLower(msg.Type.String()),
		"user.id":    agentForHttp.Session.UID(),
		"request.id": requestID.String(),
	}
	ctx = tracing.StartSpan(ctx, msg.Route, tags)
	ctx = context.WithValue(ctx, constants.SessionCtxKey, agentForHttp.Session)

	r, err := route.Decode(msg.Route)
	if err != nil {
		logger.Log.Errorf("Failed to decode route: %s", err.Error())
		return nil, err
	}

	if r.SvType == "" {
		r.SvType = h.server.Type
	}

	rm := unhandledMessage{
		ctx:   ctx,
		agent: agentForHttp,
		route: r,
		msg:   msg,
	}

	metrics.ReportMessageProcessDelayFromCtx(rm.ctx, h.metricsReporters, "remote")
	return h.remoteService.remoteProcessForHttp(rm.ctx, nil, rm.agent, rm.route, rm.msg)
}

func (h *HandlerService) localProcess(ctx context.Context, a *agent.Agent, route *route.Route, msg *message.Message) {
	var mid uint
	switch msg.Type {
	case message.Request:
		mid = msg.ID
	case message.Notify:
		mid = 0
	}

	ret, err := processHandlerMessage(ctx, route, h.serializer, a.Session, msg.Data, msg.Type, false)
	if msg.Type != message.Notify {
		if err != nil {
			logger.Log.Errorf("Failed to process handler message: %s", err.Error())
			a.AnswerWithError(ctx, mid, err)
		} else {
			err := a.Session.ResponseMID(ctx, mid, ret)
			if err != nil {
				tracing.FinishSpan(ctx, err)
				metrics.ReportTimingFromCtx(ctx, h.metricsReporters, handlerType, err)
			}
		}
	} else {
		metrics.ReportTimingFromCtx(ctx, h.metricsReporters, handlerType, nil)
		tracing.FinishSpan(ctx, err)
	}
}

// DumpServices outputs all registered services
func (h *HandlerService) DumpServices() {
	for name := range handlers {
		logger.Log.Infof("registered handler %s, isRawArg: %v", name, handlers[name].IsRawArg)
	}
}

// Docs returns documentation for handlers
func (h *HandlerService) Docs(getPtrNames bool) (map[string]interface{}, error) {
	if h == nil {
		return map[string]interface{}{}, nil
	}
	return docgenerator.HandlersDocs(h.server.Type, h.services, getPtrNames)
}

// 获取需要分配到的 dispatch id
func (h *HandlerService) GetChProcessIndex(a *agent.Agent) int {
	return int(a.Session.ID() % int64(h.dispatchNum))
}
