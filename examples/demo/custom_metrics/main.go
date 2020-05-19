package main

import (
	"flag"
	"fmt"

	"strings"

	"github.com/spf13/viper"
	"github.com/woshihaomei/pitaya"
	"github.com/woshihaomei/pitaya/acceptor"
	"github.com/woshihaomei/pitaya/component"
	"github.com/woshihaomei/pitaya/examples/demo/custom_metrics/services"
	"github.com/woshihaomei/pitaya/serialize/json"
)

func configureRoom(port int) {
	tcp := acceptor.NewTCPAcceptor(fmt.Sprintf(":%d", port))
	pitaya.AddAcceptor(tcp)

	pitaya.Register(&services.Room{},
		component.WithName("room"),
		component.WithNameFunc(strings.ToLower),
	)
}

func main() {
	port := flag.Int("port", 3250, "the port to listen")
	svType := "room"
	isFrontend := true

	flag.Parse()

	defer pitaya.Shutdown()

	pitaya.SetSerializer(json.NewSerializer())

	config := viper.New()
	config.AddConfigPath(".")
	config.SetConfigName("config")
	err := config.ReadInConfig()
	if err != nil {
		panic(err)
	}

	pitaya.Configure(isFrontend, svType, pitaya.Cluster, map[string]string{}, config)
	configureRoom(*port)
	pitaya.Start()
}
