// Kunhua Huang 2026

package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Client   ClientConfig   `yaml:"client"`
	Pool     PoolConfig     `yaml:"pool"`
	Registry RegistryConfig `yaml:"registry"`
}

type ServerConfig struct {
	Address           string        `yaml:"address"`
	Codec             string        `yaml:"codec"`
	Compress          string        `yaml:"compress"`
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout"`
	MaxConcurrent     int           `yaml:"max_concurrent"`
	WorkerPoolSize    int           `yaml:"worker_pool_size"`
	EnableRegistry    bool          `yaml:"enable_registry"`
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval"`
	Interceptors      struct {
		Recovery       bool `yaml:"recovery"`
		CircuitBreaker struct {
			Enabled          bool          `yaml:"enabled"`
			MaxRequests      uint32        `yaml:"max_requests"`
			MinRequests      uint32        `yaml:"min_requests"`
			Interval         time.Duration `yaml:"interval"`
			Timeout          time.Duration `yaml:"timeout"`
			FailureThreshold float64       `yaml:"failure_threshold"`
			SuccessThreshold uint32        `yaml:"success_threshold"`
		} `yaml:"circuit_breaker"`
	} `yaml:"interceptors"`
}

type ClientConfig struct {
	Mode           string        `yaml:"mode"` // fixed/discovery
	Address        string        `yaml:"address"`
	Timeout        time.Duration `yaml:"timeout"`
	Codec          string        `yaml:"codec"`
	Compress       string        `yaml:"compress"`
	MaxConnections int           `yaml:"max_connections"`
	MinConnections int           `yaml:"min_connections"`
	IdleTimeout    time.Duration `yaml:"idle_timeout"`
	CallTimeout    time.Duration `yaml:"call_timeout"`
	Discovery      string        `yaml:"discovery"`
	LoadBalancer   string        `yaml:"load_balancer"`
	Watch          bool          `yaml:"watch"`
	CircuitBreaker bool          `yaml:"circuit_breaker"`
}

type PoolConfig struct {
	MaxSize             int      `yaml:"max_size"`
	MinSize             int      `yaml:"min_size"`
	MaxIdleTime         Duration `yaml:"max_idle_time"`
	MaxLifetime         Duration `yaml:"max_lifetime"`
	CleanupInterval     Duration `yaml:"cleanup_interval"`
	Codec               string   `yaml:"codec"`
	Compress            string   `yaml:"compress"`
	DialTimeout         Duration `yaml:"dial_timeout"`
	KeepAlive           bool     `yaml:"keep_alive"`
	KeepAlivePeriod     Duration `yaml:"keep_alive_period"`
	EnableHealthCheck   bool     `yaml:"enable_health_check"`
	HealthCheckInterval Duration `yaml:"health_check_interval"`
	WaitTimeout         Duration `yaml:"wait_timeout"`
}

type RegistryConfig struct {
	Type string `yaml:"type"` // etcd/memory
	Etcd struct {
		Endpoints   []string `yaml:"endpoints"`
		DialTimeout Duration `yaml:"dial_timeout"`
		KeyPrefix   string   `yaml:"key_prefix"`
		LeaseTTL    int64    `yaml:"lease_ttl"`
	} `yaml:"etcd"`
}

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	dd, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = dd
	return nil
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
