// Kunhua Huang 2026

package etcd

import (
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type Config struct {
	Endpoints   []string
	DialTimeout time.Duration

	KeyPrefix string
	LeaseTTL  int64
}

func DefaultConfig() *Config {
	return &Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
		KeyPrefix:   "/rpc/services",
		LeaseTTL:    10,
	}
}

type EtcdClient struct {
	client *clientv3.Client
	config *Config
}

func NewEtcdClient(config *Config) (*EtcdClient, error) {
	if config == nil {
		config = DefaultConfig()
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   config.Endpoints,
		DialTimeout: config.DialTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("create etcd client: %w", err)
	}

	return &EtcdClient{
		client: client,
		config: config,
	}, nil
}

func (ec *EtcdClient) Close() error {
	return ec.client.Close()
}

func (ec *EtcdClient) serviceKey(service, instanceID string) string {
	return fmt.Sprintf("%s/%s/%s", ec.config.KeyPrefix, service, instanceID)
}

func (ec *EtcdClient) servicePrefix(service string) string {
	return fmt.Sprintf("%s/%s/", ec.config.KeyPrefix, service)
}
