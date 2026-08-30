// Kunhua Huang 2026

package etcd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ecstasoy/RPCinGo/pkg/registry"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type etcdWatcher struct {
	watchCh clientv3.WatchChan
	stopCh  chan struct{}
	cancel  context.CancelFunc
}

// Next blocks until the next etcd watch event can be translated into a
// registry.Event.
func (ew *etcdWatcher) Next() (*registry.Event, error) {
	for {
		select {
		case <-ew.stopCh:
			return nil, fmt.Errorf("watcher stopped")
		case watchResp, ok := <-ew.watchCh:
			if !ok {
				return nil, fmt.Errorf("watch channel closed")
			}
			if err := watchResp.Err(); err != nil {
				return nil, err
			}

			for _, ev := range watchResp.Events {
				var instance registry.ServiceInstance

				if ev.Type == clientv3.EventTypeDelete {
					if ev.PrevKv != nil {
						_ = json.Unmarshal(ev.PrevKv.Value, &instance)
					}
					return &registry.Event{Type: registry.EventTypeDelete, Instance: &instance}, nil
				}

				if err := json.Unmarshal(ev.Kv.Value, &instance); err != nil {
					fmt.Println("warning: unmarshal service instance failed:", err)
					continue
				}

				eventType := registry.EventTypeUpdate
				if ev.IsCreate() {
					eventType = registry.EventTypeAdd
				}

				return &registry.Event{Type: eventType, Instance: &instance}, nil
			}
		}
	}
}

// Stop cancels the underlying etcd watch and unblocks Next.
func (ew *etcdWatcher) Stop() {
	ew.cancel()
	close(ew.stopCh)
}
