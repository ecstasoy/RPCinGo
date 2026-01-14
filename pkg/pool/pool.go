// Kunhua Huang 2026

package pool

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"RPCinGo/pkg/protocol"
	"RPCinGo/pkg/transport"
	"RPCinGo/pkg/transport/tcp"
)

// ----------------- Pool Options & Options -----------------

type PoolOptions struct {
	MaxSize             int
	MinSize             int
	MaxIdleTime         time.Duration
	MaxLifetime         time.Duration
	CleanupInterval     time.Duration
	CodecType           protocol.CodecType
	CompressType        protocol.CompressType
	DialTimeout         time.Duration
	KeepAlive           bool
	KeepAlivePeriod     time.Duration
	EnableHealthCheck   bool
	HealthCheckInterval time.Duration

	ConnectionFactory ConnectionFactory
	Validator         PoolValidator
	WaitTimeout       time.Duration
}

func DefaultPoolOptions() *PoolOptions {
	return &PoolOptions{
		MaxSize:             100,
		MinSize:             10,
		MaxIdleTime:         90 * time.Second,
		MaxLifetime:         30 * time.Minute,
		CleanupInterval:     30 * time.Second,
		CodecType:           protocol.CodecTypeJSON,
		CompressType:        protocol.CompressTypeNone,
		DialTimeout:         5 * time.Second,
		KeepAlive:           true,
		KeepAlivePeriod:     30 * time.Second,
		EnableHealthCheck:   true,
		HealthCheckInterval: 60 * time.Second,
		WaitTimeout:         5 * time.Second,
	}
}

type PoolOption func(*PoolOptions)

func WithPoolSize(max, min int) PoolOption {
	return func(opts *PoolOptions) {
		opts.MaxSize = max
		opts.MinSize = min
	}
}

func WithIdleTimeout(timeout time.Duration) PoolOption {
	return func(opts *PoolOptions) {
		opts.MaxIdleTime = timeout
	}
}

func WithMaxLifetime(lifetime time.Duration) PoolOption {
	return func(opts *PoolOptions) {
		opts.MaxLifetime = lifetime
	}
}

func WithCleanupInterval(interval time.Duration) PoolOption {
	return func(opts *PoolOptions) {
		opts.CleanupInterval = interval
	}
}

func WithPoolCodec(codecType protocol.CodecType, compressType protocol.CompressType) PoolOption {
	return func(opts *PoolOptions) {
		opts.CodecType = codecType
		opts.CompressType = compressType
	}
}

func WithHealthCheck(enable bool, interval time.Duration) PoolOption {
	return func(opts *PoolOptions) {
		opts.EnableHealthCheck = enable
		opts.HealthCheckInterval = interval
	}
}

func WithConnectionFactory(factory ConnectionFactory) PoolOption {
	return func(opts *PoolOptions) {
		opts.ConnectionFactory = factory
	}
}

func WithPoolValidator(validator PoolValidator) PoolOption {
	return func(opts *PoolOptions) {
		opts.Validator = validator
	}
}

func WithWaitTimeout(timeout time.Duration) PoolOption {
	return func(opts *PoolOptions) {
		opts.WaitTimeout = timeout
	}
}

// ----------------- Connection Factory -----------------

type ConnectionFactory interface {
	Create(address string) (*tcp.Client, error)
	Validate() error
}

type DefaultConnectionFactory struct {
	codecType       protocol.CodecType
	compressType    protocol.CompressType
	dialTimeout     time.Duration
	keepAlive       bool
	keepAlivePeriod time.Duration
	clientOptions   []transport.ClientOption
}

func NewDefaultConnectionFactory(
	codecType protocol.CodecType,
	compressType protocol.CompressType,
	dialTimeout time.Duration,
) ConnectionFactory {
	return &DefaultConnectionFactory{
		codecType:       codecType,
		compressType:    compressType,
		dialTimeout:     dialTimeout,
		keepAlive:       true,
		keepAlivePeriod: 30 * time.Second,
	}
}

func (f *DefaultConnectionFactory) Create(address string) (*tcp.Client, error) {
	final := append([]transport.ClientOption{}, f.clientOptions...)
	final = append(final,
		transport.WithDialTimeout(f.dialTimeout),
		transport.WithKeepAlive(f.keepAlive, f.keepAlivePeriod),
	)

	client := tcp.NewClient(address, f.codecType, f.compressType, final...)

	ctx, cancel := context.WithTimeout(context.Background(), f.dialTimeout)
	defer cancel()

	if err := client.Dial(ctx, ""); err != nil {
		return nil, fmt.Errorf("dial failed: %w", err)
	}

	return client, nil
}

func (f *DefaultConnectionFactory) Validate() error {
	if f.dialTimeout <= 0 {
		return fmt.Errorf("dialTimeout must be > 0")
	}
	return nil
}

// ClientOptions allows setting additional Client options.

func (f *DefaultConnectionFactory) SetClientOptions(opts ...transport.ClientOption) {
	f.clientOptions = opts
}

// ----------------- Retry Connection Factory -----------------

type RetryConnectionFactory struct {
	baseFactory   ConnectionFactory
	maxRetries    int
	retryInterval time.Duration
}

func NewRetryConnectionFactory(
	baseFactory ConnectionFactory,
	maxRetries int,
	retryInterval time.Duration,
) ConnectionFactory {
	return &RetryConnectionFactory{
		baseFactory:   baseFactory,
		maxRetries:    maxRetries,
		retryInterval: retryInterval,
	}
}

func (f *RetryConnectionFactory) Create(address string) (*tcp.Client, error) {
	var lastErr error

	for attempt := 0; attempt <= f.maxRetries; attempt++ {
		client, err := f.baseFactory.Create(address)
		if err == nil {
			return client, nil
		}

		lastErr = err

		if attempt < f.maxRetries {
			time.Sleep(f.retryInterval)
		}
	}

	return nil, fmt.Errorf("create connection failed after %d retries: %w",
		f.maxRetries, lastErr)
}

func (f *RetryConnectionFactory) Validate() error {
	if f.baseFactory == nil {
		return fmt.Errorf("baseFactory is nil")
	}
	return f.baseFactory.Validate()
}

// ------------------ Mock Connection Factory ------------------

type MockConnectionFactory struct {
	createFunc func(address string) (*tcp.Client, error)
	callCount  int
	mu         sync.Mutex
}

func NewMockConnectionFactory(createFunc func(string) (*tcp.Client, error)) ConnectionFactory {
	return &MockConnectionFactory{
		createFunc: createFunc,
	}
}

func (f *MockConnectionFactory) Create(address string) (*tcp.Client, error) {
	f.mu.Lock()
	f.callCount++
	f.mu.Unlock()

	if f.createFunc != nil {
		return f.createFunc(address)
	}

	return nil, fmt.Errorf("mock: no Client")
}

func (f *MockConnectionFactory) Validate() error {
	return nil
}

func (f *MockConnectionFactory) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.callCount
}

// ----------------- Pool Validator -----------------

type PoolValidator interface {
	Validate(opts *PoolOptions) error
}

type DefaultPoolValidator struct{}

func (v *DefaultPoolValidator) Validate(opts *PoolOptions) error {
	if opts.MaxSize <= 0 {
		return fmt.Errorf("MaxSize must be > 0, got %d", opts.MaxSize)
	}

	if opts.MinSize < 0 {
		return fmt.Errorf("MinSize must be >= 0, got %d", opts.MinSize)
	}

	if opts.MinSize > opts.MaxSize {
		return fmt.Errorf("MinSize (%d) must be <= MaxSize (%d)",
			opts.MinSize, opts.MaxSize)
	}

	if opts.MinSize > opts.MaxSize/2 {
		fmt.Printf("Warning: MinSize (%d) > MaxSize/2, may waste resources\n",
			opts.MinSize)
	}

	if opts.MaxIdleTime <= 0 {
		return fmt.Errorf("MaxIdleTime must be > 0, got %v", opts.MaxIdleTime)
	}

	if opts.MaxLifetime < 0 {
		return fmt.Errorf("MaxLifetime must be >= 0 (0 means unlimited), got %v",
			opts.MaxLifetime)
	}

	if opts.MaxLifetime > 0 && opts.MaxLifetime < opts.MaxIdleTime {
		return fmt.Errorf("MaxLifetime (%v) should be >= MaxIdleTime (%v)",
			opts.MaxLifetime, opts.MaxIdleTime)
	}

	if opts.CleanupInterval <= 0 {
		return fmt.Errorf("CleanupInterval must be > 0, got %v", opts.CleanupInterval)
	}

	if opts.CleanupInterval > opts.MaxIdleTime {
		return fmt.Errorf("CleanupInterval (%v) should be <= MaxIdleTime (%v)",
			opts.CleanupInterval, opts.MaxIdleTime)
	}

	if opts.CleanupInterval < 5*time.Second {
		fmt.Printf("Warning: CleanupInterval (%v) is too short, may impact performance\n",
			opts.CleanupInterval)
	}

	if opts.DialTimeout <= 0 {
		return fmt.Errorf("DialTimeout must be > 0, got %v", opts.DialTimeout)
	}

	if opts.KeepAlive && opts.KeepAlivePeriod <= 0 {
		return fmt.Errorf("KeepAlivePeriod must be > 0 when KeepAlive is enabled, got %v",
			opts.KeepAlivePeriod)
	}

	if opts.EnableHealthCheck && opts.HealthCheckInterval <= 0 {
		return fmt.Errorf("HealthCheckInterval must be > 0 when health check is enabled, got %v",
			opts.HealthCheckInterval)
	}

	if opts.WaitTimeout < 0 {
		return fmt.Errorf("WaitTimeout must be >= 0 (0 means wait forever), got %v",
			opts.WaitTimeout)
	}

	switch opts.CodecType {
	case protocol.CodecTypeJSON, protocol.CodecTypeProtobuf, protocol.CodecTypeMsgPack:
	default:
		return fmt.Errorf("unsupported CodecType: %v", opts.CodecType)
	}

	return nil
}

// ----------------- Pooled Connection -----------------

type PooledConnection struct {
	Client    *tcp.Client
	pool      *ConnectionPool
	createdAt time.Time
	lastUsed  time.Time
	useCount  int
	closed    bool
	mu        sync.Mutex
}

func newPooledConnection(client *tcp.Client, pool *ConnectionPool) *PooledConnection {
	now := time.Now()
	return &PooledConnection{
		Client:    client,
		pool:      pool,
		createdAt: now,
		lastUsed:  now,
		useCount:  0,
	}
}

func (pc *PooledConnection) Send(ctc context.Context, data []byte) ([]byte, error) {
	pc.mu.Lock()

	if pc.closed {
		pc.mu.Unlock()
		return nil, fmt.Errorf("connection is closed")
	}

	pc.lastUsed = time.Now()
	pc.useCount++

	pc.mu.Unlock()

	return pc.Client.Send(ctc, data)
}

func (pc *PooledConnection) IsHealthy() bool {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.closed {
		return false
	}

	if !pc.Client.IsConnected() {
		return false
	}

	return true
}

func (pc *PooledConnection) IsExpired(maxIdleTime, maxLifetime time.Duration) bool {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	now := time.Now()

	if now.Sub(pc.lastUsed) > maxIdleTime {
		return true
	}

	if maxLifetime > 0 && now.Sub(pc.createdAt) > maxLifetime {
		return true
	}

	return false
}

func (pc *PooledConnection) Close() error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.closed {
		return nil
	}

	pc.closed = true

	if pc.Client != nil {
		return pc.Client.Close()
	}

	return nil
}

func (pc *PooledConnection) LastUsed() time.Time {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	return pc.lastUsed
}

func (pc *PooledConnection) Release() {
	if pc.pool != nil {
		pc.pool.Put(pc)
	}
}

func (pc *PooledConnection) Stats() ConnectionStats {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	return ConnectionStats{
		CreatedAt: pc.createdAt,
		LastUsed:  pc.lastUsed,
		UseCount:  int64(pc.useCount),
		Age:       time.Since(pc.createdAt),
		IdleTime:  time.Since(pc.lastUsed),
	}
}

type ConnectionStats struct {
	CreatedAt time.Time
	LastUsed  time.Time
	UseCount  int64
	Age       time.Duration
	IdleTime  time.Duration
}

// ----------------- Connection Pool -----------------

type ConnectionPool struct {
	address     string
	opts        *PoolOptions
	pool        chan *PooledConnection
	factory     ConnectionFactory
	mu          sync.RWMutex
	currentSize int
	closed      bool
	// cleanup routine
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}

	stats struct {
		getCount    int64
		putCount    int64
		createCount int64
		closeCount  int64
	}
}

func NewConnectionPool(address string, options ...PoolOption) (*ConnectionPool, error) {
	opts := DefaultPoolOptions()
	for _, opt := range options {
		opt(opts)
	}

	validator := opts.Validator
	if validator == nil {
		validator = &DefaultPoolValidator{}
	}

	if err := validator.Validate(opts); err != nil {
		return nil, fmt.Errorf("invalid pool options: %w", err)
	}

	factory := opts.ConnectionFactory
	if factory == nil {
		factory = NewDefaultConnectionFactory(
			opts.CodecType,
			opts.CompressType,
			opts.DialTimeout,
		)
	}

	if err := factory.Validate(); err != nil {
		return nil, fmt.Errorf("invalid connection factory: %w", err)
	}

	pool := &ConnectionPool{
		address:       address,
		opts:          opts,
		factory:       factory,
		pool:          make(chan *PooledConnection, opts.MaxSize),
		stopCleanup:   make(chan struct{}),
		cleanupTicker: time.NewTicker(opts.CleanupInterval),
	}

	// Pre-create minimum number of connections
	for i := 0; i < opts.MinSize; i++ {
		conn, err := pool.createConnection()
		if err != nil {
			// pre-creation failed, just continue
			// TODO: Logging system can be used here
			fmt.Println("Warning: pre-creation of connection failed:", err)
			break
		}
		pool.pool <- conn
		pool.currentSize++
		atomic.AddInt64(&pool.stats.createCount, 1)
	}

	go pool.cleanupRoutine()
	if opts.EnableHealthCheck {
		go pool.healthCheckRoutine()
	}
	return pool, nil
}

func (p *ConnectionPool) createConnection() (*PooledConnection, error) {
	client, err := p.factory.Create(p.address)
	if err != nil {
		return nil, fmt.Errorf("factory create failed: %w", err)
	}

	return newPooledConnection(client, p), nil
}

func (p *ConnectionPool) Get() (*PooledConnection, error) {
	return p.GetWithContext(context.Background())
}
func (p *ConnectionPool) GetWithContext(ctx context.Context) (*PooledConnection, error) {
	atomic.AddInt64(&p.stats.getCount, 1)

	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, fmt.Errorf("connection pool is closed")
	}
	p.mu.RUnlock()

	select {
	case conn := <-p.pool:
		if conn.IsHealthy() && !conn.IsExpired(p.opts.MaxIdleTime, p.opts.MaxLifetime) {
			return conn, nil
		}

		conn.Close()
		atomic.AddInt64(&p.stats.closeCount, 1)

		p.mu.Lock()
		p.currentSize--
		p.mu.Unlock()

		return p.createNewConnectionWithContext(ctx)
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return p.createNewConnectionWithContext(ctx)
	}
}

func (p *ConnectionPool) createNewConnectionWithContext(ctx context.Context) (*PooledConnection, error) {
	for {
		p.mu.Lock()

		if p.currentSize >= p.opts.MaxSize {
			p.mu.Unlock()

			timeout := p.opts.WaitTimeout
			if timeout <= 0 {
				timeout = 5 * time.Second
			}

			waitCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			select {
			case conn := <-p.pool:
				if conn.IsHealthy() && !conn.IsExpired(p.opts.MaxIdleTime, p.opts.MaxLifetime) {
					return conn, nil
				}

				conn.Close()
				atomic.AddInt64(&p.stats.closeCount, 1)

				p.mu.Lock()
				p.currentSize--
				p.mu.Unlock()

				continue
			case <-waitCtx.Done():
				return nil, fmt.Errorf("wait for connection timeout after %v: %w", timeout, waitCtx.Err())
			}

		}
		p.mu.Unlock()

		conn, err := p.createConnection()
		if err != nil {
			return nil, err
		}

		p.mu.Lock()
		p.currentSize++
		p.mu.Unlock()

		atomic.AddInt64(&p.stats.createCount, 1)

		return conn, nil
	}
}

func (p *ConnectionPool) Put(conn *PooledConnection) {
	if conn == nil {
		return
	}

	atomic.AddInt64(&p.stats.putCount, 1)

	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		conn.Close()
		atomic.AddInt64(&p.stats.closeCount, 1)
		return
	}
	p.mu.RUnlock()

	if !conn.IsHealthy() || conn.IsExpired(p.opts.MaxIdleTime, p.opts.MaxLifetime) {
		conn.Close()
		atomic.AddInt64(&p.stats.closeCount, 1)

		p.mu.Lock()
		p.currentSize--
		p.mu.Unlock()
		return
	}

	select {
	case p.pool <- conn:
	default:
		conn.Close()
		atomic.AddInt64(&p.stats.closeCount, 1)

		p.mu.Lock()
		p.currentSize--
		p.mu.Unlock()
	}
}

func (p *ConnectionPool) cleanupRoutine() {
	for {
		select {
		case <-p.cleanupTicker.C:
			p.cleanup()
		case <-p.stopCleanup:
			return
		}
	}
}

func (p *ConnectionPool) cleanup() {
	var toCheck []*PooledConnection

	for {
		select {
		case conn := <-p.pool:
			toCheck = append(toCheck, conn)
		default:
			goto check
		}
	}

check:
	keepCount := 0
	for _, conn := range toCheck {
		if conn.IsExpired(p.opts.MaxIdleTime, p.opts.MaxLifetime) {
			conn.Close()
			atomic.AddInt64(&p.stats.closeCount, 1)

			p.mu.Lock()
			p.currentSize--
			p.mu.Unlock()
		} else {
			p.pool <- conn
			keepCount++
		}
	}

	p.mu.Lock()
	if p.currentSize < p.opts.MinSize {
		toCreate := p.opts.MinSize - p.currentSize
		for i := 0; i < toCreate; i++ {
			conn, err := p.createConnection()
			if err != nil {
				break
			}
			p.pool <- conn
			p.currentSize++
			atomic.AddInt64(&p.stats.createCount, 1)
		}
	}
	p.mu.Unlock()
}

func (p *ConnectionPool) healthCheckRoutine() {
	ticker := time.NewTicker(p.opts.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.healthCheck()
		case <-p.stopCleanup:
			return
		}
	}
}

func (p *ConnectionPool) healthCheck() {
	var toCheck []*PooledConnection

	for len(p.pool) > 0 {
		select {
		case conn := <-p.pool:
			toCheck = append(toCheck, conn)
		default:
			goto check
		}
	}

check:
	for _, conn := range toCheck {
		if conn.IsHealthy() {
			p.pool <- conn
		} else {
			conn.Close()
			atomic.AddInt64(&p.stats.closeCount, 1)

			p.mu.Lock()
			p.currentSize--
			p.mu.Unlock()
		}
	}
}

func (p *ConnectionPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true

	close(p.stopCleanup)
	p.cleanupTicker.Stop()

	close(p.pool)

	for conn := range p.pool {
		conn.Close()
		atomic.AddInt64(&p.stats.closeCount, 1)
	}

	return nil
}

type PoolStats struct {
	Address     string
	CurrentSize int
	IdleSize    int
	MaxSize     int
	MinSize     int
	GetCount    int64
	PutCount    int64
	CreateCount int64
	CloseCount  int64
}

func (p *ConnectionPool) Stats() PoolStats {
	p.mu.RLock()
	currentSize := p.currentSize
	idleSize := len(p.pool)
	p.mu.RUnlock()

	return PoolStats{
		Address:     p.address,
		CurrentSize: currentSize,
		IdleSize:    idleSize,
		MaxSize:     p.opts.MaxSize,
		MinSize:     p.opts.MinSize,
		GetCount:    atomic.LoadInt64(&p.stats.getCount),
		PutCount:    atomic.LoadInt64(&p.stats.putCount),
		CreateCount: atomic.LoadInt64(&p.stats.createCount),
		CloseCount:  atomic.LoadInt64(&p.stats.closeCount),
	}
}

func (s PoolStats) String() string {
	return fmt.Sprintf("Pool{Addr=%s, Size=%d/%d (idle=%d), "+
		"Get=%d, Put=%d, Create=%d, Close=%d}",
		s.Address, s.CurrentSize, s.MaxSize, s.IdleSize,
		s.GetCount, s.PutCount, s.CreateCount, s.CloseCount)
}
