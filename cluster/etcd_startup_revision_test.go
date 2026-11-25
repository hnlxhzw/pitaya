package cluster

import (
	"os"
	"testing"
	"time"

	"github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/namespace"
)

// Run against a disposable etcd. Insert the frontend specifically after the
// initial snapshot but before starting the watch; no periodic resync is run.
func TestWatchReplaysFrontendRegisteredAfterSnapshot(t *testing.T) {
	addr := os.Getenv("PITAYA_TEST_ETCD_ENDPOINT")
	if addr == "" {
		t.Skip("set PITAYA_TEST_ETCD_ENDPOINT to a disposable etcd")
	}
	cli, err := clientv3.New(clientv3.Config{Endpoints: []string{addr}, DialTimeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	prefix := "pitaya-discovery-test/" + time.Now().Format("150405.000000000") + "/"
	cli.KV = namespace.NewKV(cli.KV, prefix)
	cli.Watcher = namespace.NewWatcher(cli.Watcher, prefix)
	sd := &etcdServiceDiscovery{cli: cli, server: &Server{ID: "local", Type: "game"}, serverMapByType: make(map[string]map[string]*Server), stopChan: make(chan bool), running: true}
	defer close(sd.stopChan)
	if err = sd.bootstrapServer(sd.server); err != nil {
		t.Fatal(err)
	}
	peer := &Server{ID: "late-frontend", Type: "connector", Frontend: true}
	if err = sd.addServerIntoEtcd(peer); err != nil {
		t.Fatal(err)
	}
	sd.watchEtcdChanges()
	until := time.Now().Add(2 * time.Second)
	for time.Now().Before(until) {
		if found, e := sd.GetServer(peer.ID); e == nil && found.Type == peer.Type {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("frontend registration between snapshot and watch was lost")
}
