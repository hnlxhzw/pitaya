package main

import (
	"flag"
	"fmt"

	"strings"

	"github.com/hnlxhzw/pitaya"
	"github.com/hnlxhzw/pitaya/acceptor"
	"github.com/hnlxhzw/pitaya/component"
	"github.com/hnlxhzw/pitaya/examples/demo/cluster_protobuf/services"
	"github.com/hnlxhzw/pitaya/serialize/protobuf"
)

func configureBackend() {
	room := services.NewRoom()
	pitaya.Register(room,
		component.WithName("room"),
		component.WithNameFunc(strings.ToLower),
	)

	pitaya.RegisterRemote(room,
		component.WithName("room"),
		component.WithNameFunc(strings.ToLower),
	)
}

func configureFrontend(port int) {
	ws := acceptor.NewWSAcceptor(fmt.Sprintf(":%d", port))
	pitaya.Register(&services.Connector{},
		component.WithName("connector"),
		component.WithNameFunc(strings.ToLower),
	)
	pitaya.RegisterRemote(&services.ConnectorRemote{},
		component.WithName("connectorremote"),
		component.WithNameFunc(strings.ToLower),
	)

	pitaya.AddAcceptor(ws)
}

func main() {
	port := flag.Int("port", 3250, "the port to listen")
	svType := flag.String("type", "connector", "the server type")
	isFrontend := flag.Bool("frontend", true, "if server is frontend")

	flag.Parse()

	defer pitaya.Shutdown()

	ser := protobuf.NewSerializer()

	pitaya.SetSerializer(ser)

	if !*isFrontend {
		configureBackend()
	} else {
		configureFrontend(*port)
	}

	pitaya.Configure(*isFrontend, *svType, pitaya.Cluster, map[string]string{})
	pitaya.Start()
}
