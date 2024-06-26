//
// Copyright (c) TFG Co. All Rights Reserved.
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
	"errors"
	"fmt"
	"reflect"

	"github.com/golang/protobuf/proto"

	"github.com/hnlxhzw/pitaya/agent"
	"github.com/hnlxhzw/pitaya/cluster"
	"github.com/hnlxhzw/pitaya/component"
	"github.com/hnlxhzw/pitaya/conn/codec"
	"github.com/hnlxhzw/pitaya/conn/message"
	"github.com/hnlxhzw/pitaya/constants"
	"github.com/hnlxhzw/pitaya/docgenerator"
	e "github.com/hnlxhzw/pitaya/errors"
	"github.com/hnlxhzw/pitaya/logger"
	"github.com/hnlxhzw/pitaya/protos"
	"github.com/hnlxhzw/pitaya/route"
	"github.com/hnlxhzw/pitaya/router"
	"github.com/hnlxhzw/pitaya/serialize"
	"github.com/hnlxhzw/pitaya/session"
	"github.com/hnlxhzw/pitaya/tracing"
	"github.com/hnlxhzw/pitaya/util"
)

// RemoteService struct
type RemoteService struct {
	rpcServer              cluster.RPCServer
	serviceDiscovery       cluster.ServiceDiscovery
	serializer             serialize.Serializer
	encoder                codec.PacketEncoder
	rpcClient              cluster.RPCClient
	services               map[string]*component.Service // all registered service
	router                 *router.Router
	messageEncoder         message.Encoder
	server                 *cluster.Server // server obj
	remoteBindingListeners []cluster.RemoteBindingListener
}

// NewRemoteService creates and return a new RemoteService
func NewRemoteService(
	rpcClient cluster.RPCClient,
	rpcServer cluster.RPCServer,
	sd cluster.ServiceDiscovery,
	encoder codec.PacketEncoder,
	serializer serialize.Serializer,
	router *router.Router,
	messageEncoder message.Encoder,
	server *cluster.Server,
) *RemoteService {
	return &RemoteService{
		services:               make(map[string]*component.Service),
		rpcClient:              rpcClient,
		rpcServer:              rpcServer,
		encoder:                encoder,
		serviceDiscovery:       sd,
		serializer:             serializer,
		router:                 router,
		messageEncoder:         messageEncoder,
		server:                 server,
		remoteBindingListeners: make([]cluster.RemoteBindingListener, 0),
	}
}

var remotes = make(map[string]*component.Remote) // all remote method

func (r *RemoteService) remoteProcess(
	ctx context.Context,
	server *cluster.Server,
	a *agent.Agent,
	route *route.Route,
	msg *message.Message,
) {
	res, err := r.remoteCall(ctx, server, protos.RPCType_Sys, route, a.Session, msg)
	switch msg.Type {
	case message.Request:
		if err != nil {
			logger.Log.Errorf("Failed to process remote server: %s", err.Error())
			a.AnswerWithError(ctx, msg.ID, err)
			return
		}
		err := a.Session.ResponseMID(ctx, msg.ID, res.Data)
		if err != nil {
			logger.Log.Errorf("Failed to respond to remote server: %s", err.Error())
			a.AnswerWithError(ctx, msg.ID, err)
		}
	case message.Notify:
		defer tracing.FinishSpan(ctx, err)
		if err == nil && res.Error != nil {
			err = errors.New(res.Error.GetMsg())
		}
		if err != nil {
			logger.Log.Errorf("error while sending a notify to server: %s", err.Error())
		}
	}
}

func (r *RemoteService) remoteProcessForHttp(
	ctx context.Context,
	server *cluster.Server,
	a *agent.Agent,
	route *route.Route,
	msg *message.Message,
) (*protos.Response, error) {
	res, err := r.remoteCall(ctx, server, protos.RPCType_Sys, route, a.Session, msg)
	return res, err
}

// AddRemoteBindingListener adds a listener
func (r *RemoteService) AddRemoteBindingListener(bindingListener cluster.RemoteBindingListener) {
	r.remoteBindingListeners = append(r.remoteBindingListeners, bindingListener)
}

// Call processes a remote call
func (r *RemoteService) Call(ctx context.Context, req *protos.Request) (*protos.Response, error) {
	defer util.AutoRecover("Call")

	c, err := util.GetContextFromRequest(req, r.server.ID)
	c = util.StartSpanFromRequest(c, r.server.ID, req.GetMsg().GetRoute())
	var res *protos.Response
	if err != nil {
		res = &protos.Response{
			Error: &protos.Error{
				Code:      e.ErrInternalCode.Desc,
				ErrorCode: e.ErrInternalCode.ErrorCode,
				Msg:       err.Error(),
			},
		}
	} else {
		res = processRemoteMessage(c, req, r)
	}

	if res.Error != nil {
		err = errors.New(res.Error.Msg)
	}

	defer tracing.FinishSpan(c, err)
	return res, nil
}

// SessionBindRemote is called when a remote server binds a user session and want us to acknowledge it
func (r *RemoteService) SessionBindRemote(ctx context.Context, msg *protos.BindMsg) (*protos.Response, error) {
	defer util.AutoRecover("SessionBindRemote")

	for _, r := range r.remoteBindingListeners {
		r.OnUserBind(msg.Uid, msg.Fid)
	}
	return &protos.Response{
		Data: []byte("ack"),
	}, nil
}

// PushToUser sends a push to user
func (r *RemoteService) PushToUser(ctx context.Context, push *protos.Push) (*protos.Response, error) {
	defer util.AutoRecover("PushToUser")
	logger.Log.Debugf("sending push to user %s: %v", push.GetUid(), string(push.Data))
	s := session.GetSessionByUID(push.GetUid())
	if s != nil {
		err := s.Push(push.Route, push.Data)
		if err != nil {
			return nil, err
		}
		return &protos.Response{
			Data: []byte("ack"),
		}, nil
	}
	return nil, constants.ErrSessionNotFound
}

// KickUser sends a kick to user
func (r *RemoteService) KickUser(ctx context.Context, kick *protos.KickMsg) (*protos.KickAnswer, error) {
	defer util.AutoRecover("KickUser")

	logger.Log.Debugf("sending kick to user %s", kick.GetUserId())
	s := session.GetSessionByUID(kick.GetUserId())
	if s != nil {
		err := s.Kick(ctx)
		if err != nil {
			return nil, err
		}
		return &protos.KickAnswer{
			Kicked: true,
		}, nil
	}
	return nil, constants.ErrSessionNotFound
}

// DoRPC do rpc and get answer
func (r *RemoteService) DoRPC(ctx context.Context, serverID string, route *route.Route, protoData []byte) (*protos.Response, error) {
	msg := &message.Message{
		Type:  message.Request,
		Route: route.Short(),
		Data:  protoData,
	}

	target, _ := r.serviceDiscovery.GetServer(serverID)
	if serverID != "" && target == nil {
		return nil, constants.ErrServerNotFound
	}

	return r.remoteCall(ctx, target, protos.RPCType_User, route, nil, msg)
}

func (r *RemoteService) DoGRPC(ctx context.Context, serverID string, routeKey string, req interface{}, resp interface{}) error {
	target, _ := r.serviceDiscovery.GetServer(serverID)
	if serverID != "" && target == nil {
		return constants.ErrServerNotFound
	}
	return r.rpcClient.(*cluster.GRPCClient).Call2(ctx, routeKey, req, resp, target)
}

// RPC makes rpcs
func (r *RemoteService) RPC(ctx context.Context, serverID string, route *route.Route, reply proto.Message, arg proto.Message) error {
	defer util.AutoRecover("RemoteService RPC")

	var data []byte
	var err error
	if arg != nil {
		data, err = proto.Marshal(arg)
		if err != nil {
			return err
		}
	}
	res, err := r.DoRPC(ctx, serverID, route, data)
	if err != nil {
		return err
	}

	if res.Error != nil {
		return &e.Error{
			Code:     res.Error.Code,
			Message:  res.Error.Msg,
			Metadata: res.Error.Metadata,
		}
	}

	if reply != nil {
		err = proto.Unmarshal(res.GetData(), reply)
		if err != nil {
			return err
		}
	}
	return nil
}

// Register registers components
func (r *RemoteService) Register(comp component.Component, opts []component.Option) error {
	s := component.NewService(comp, opts)

	if _, ok := r.services[s.Name]; ok {
		return fmt.Errorf("remote: service already defined: %s", s.Name)
	}

	if err := s.ExtractRemote(); err != nil {
		return err
	}

	r.services[s.Name] = s
	// register all remotes
	for name, remote := range s.Remotes {
		remotes[fmt.Sprintf("%s.%s", s.Name, name)] = remote
	}

	return nil
}

func processRemoteMessage(ctx context.Context, req *protos.Request, r *RemoteService) *protos.Response {
	rt, err := route.Decode(req.GetMsg().GetRoute())
	if err != nil {
		response := &protos.Response{
			Error: &protos.Error{
				Code:      e.ErrBadRequestCode.Desc,
				ErrorCode: e.ErrBadRequestCode.ErrorCode,
				Msg:       "cannot decode route",
				Metadata: map[string]string{
					"route": req.GetMsg().GetRoute(),
				},
			},
		}
		return response
	}

	switch {
	case req.Type == protos.RPCType_Sys:
		return r.handleRPCSys(ctx, req, rt)
	case req.Type == protos.RPCType_User:
		return r.handleRPCUser(ctx, req, rt)
	default:
		return &protos.Response{
			Error: &protos.Error{
				Code:      e.ErrBadRequestCode.Desc,
				ErrorCode: e.ErrBadRequestCode.ErrorCode,
				Msg:       "invalid rpc type",
				Metadata: map[string]string{
					"route": req.GetMsg().GetRoute(),
				},
			},
		}
	}
}

func (r *RemoteService) handleRPCUser(ctx context.Context, req *protos.Request, rt *route.Route) *protos.Response {
	response := &protos.Response{}

	remote, ok := remotes[rt.Short()]
	if !ok {
		logger.Log.Warnf("pitaya/remote: %s not found", rt.Short())
		response := &protos.Response{
			Error: &protos.Error{
				Code:      e.ErrNotFoundCode.Desc,
				ErrorCode: e.ErrNotFoundCode.ErrorCode,
				Msg:       "route not found",
				Metadata: map[string]string{
					"route": rt.Short(),
				},
			},
		}
		return response
	}
	params := []reflect.Value{remote.Receiver, reflect.ValueOf(ctx)}
	if remote.HasArgs {
		arg, err := unmarshalRemoteArg(remote, req.GetMsg().GetData())
		if err != nil {
			response := &protos.Response{
				Error: &protos.Error{
					Code:      e.ErrBadRequestCode.Desc,
					ErrorCode: e.ErrBadRequestCode.ErrorCode,
					Msg:       err.Error(),
				},
			}
			return response
		}
		params = append(params, reflect.ValueOf(arg))
	}

	ret, err := util.Pcall(remote.Method, params)
	if err != nil {
		response := &protos.Response{
			Error: &protos.Error{
				Code:      e.ErrUnknownCode.Desc,
				ErrorCode: e.ErrUnknownCode.ErrorCode,
				Msg:       err.Error(),
			},
		}
		if val, ok := err.(*e.Error); ok {
			response.Error.Code = val.Code
			if val.Metadata != nil {
				response.Error.Metadata = val.Metadata
			}
		}
		return response
	}

	var b []byte
	if ret != nil {
		pb, ok := ret.(proto.Message)
		if !ok {
			response := &protos.Response{
				Error: &protos.Error{
					Code:      e.ErrUnknownCode.Desc,
					ErrorCode: e.ErrUnknownCode.ErrorCode,
					Msg:       constants.ErrWrongValueType.Error(),
				},
			}
			return response
		}
		if b, err = proto.Marshal(pb); err != nil {
			response := &protos.Response{
				Error: &protos.Error{
					Code:      e.ErrUnknownCode.Desc,
					ErrorCode: e.ErrUnknownCode.ErrorCode,
					Msg:       err.Error(),
				},
			}
			return response
		}
	}

	response.Data = b
	return response
}

func (r *RemoteService) handleRPCSys(ctx context.Context, req *protos.Request, rt *route.Route) *protos.Response {
	reply := req.GetMsg().GetReply()
	response := &protos.Response{}
	// (warning) a new agent is created for every new request
	a, err := agent.NewRemote(
		req.GetSession(),
		reply,
		r.rpcClient,
		r.encoder,
		r.serializer,
		r.serviceDiscovery,
		req.FrontendID,
		r.messageEncoder,
	)
	if err != nil {
		logger.Log.Warn("pitaya/handler: cannot instantiate remote agent")
		response := &protos.Response{
			Error: &protos.Error{
				Code:      e.ErrInternalCode.Desc,
				ErrorCode: e.ErrInternalCode.ErrorCode,
				Msg:       err.Error(),
			},
		}
		return response
	}

	ret, err := processHandlerMessage(ctx, rt, r.serializer, a.Session, req.GetMsg().GetData(), req.GetMsg().GetType(), true)
	if err != nil {
		logger.Log.Warnf(err.Error())
		response = &protos.Response{
			Error: &protos.Error{
				Code:      e.ErrUnknownCode.Desc,
				ErrorCode: e.ErrUnknownCode.ErrorCode,
				Msg:       err.Error(),
			},
		}
		if val, ok := err.(*e.Error); ok {
			response.Error.Code = val.Code
			response.Error.ErrorCode = val.ErrorCode
			if val.Metadata != nil {
				response.Error.Metadata = val.Metadata
			}
		}
	} else {
		response = &protos.Response{Data: ret}
	}
	return response
}

func (r *RemoteService) remoteCall(
	ctx context.Context,
	server *cluster.Server,
	rpcType protos.RPCType,
	route *route.Route,
	session *session.Session,
	msg *message.Message,
) (*protos.Response, error) {
	svType := route.SvType

	var err error
	target := server

	if target == nil {
		target, err = r.router.Route(ctx, rpcType, svType, route, msg)
		if err != nil {
			return nil, e.NewError(err, e.ErrInternalCode.Desc, e.ErrInternalCode.ErrorCode)
		}
	}

	res, err := r.rpcClient.Call(ctx, rpcType, route, session, msg, target)
	if err != nil {
		if err, ok := err.(*e.Error); ok {
			return nil, e.NewError(
				fmt.Errorf("error making call to target with id %s and host %s: %s", target.ID, target.Hostname, err.Message),
				err.Code,
				err.ErrorCode,
				err.Metadata,
			)
		}

		return nil, fmt.Errorf("error making call to target with id %s and host %s: %w", target.ID, target.Hostname, err)
	}
	return res, err
}

// DumpServices outputs all registered services
func (r *RemoteService) DumpServices() {
	for name := range remotes {
		logger.Log.Infof("registered remote %s", name)
	}
}

// Docs returns documentation for remotes
func (r *RemoteService) Docs(getPtrNames bool) (map[string]interface{}, error) {
	if r == nil {
		return map[string]interface{}{}, nil
	}
	return docgenerator.RemotesDocs(r.server.Type, r.services, getPtrNames)
}
