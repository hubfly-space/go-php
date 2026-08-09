package fastcgi

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrPoolClosed    = errors.New("fastcgi: pool is closed")
	ErrPoolExhausted = errors.New("fastcgi: connection pool limit reached")
)

// PoolConfig configures FastCGI connection pooling behavior.
type PoolConfig struct {
	Network     string
	Address     string
	MaxIdle     int
	MaxActive   int
	IdleTimeout time.Duration
}

type pooledClient struct {
	client   *Client
	lastUsed time.Time
}

// Pool manages reusable FastCGI client connections.
type Pool struct {
	network     string
	address     string
	maxIdle     int
	maxActive   int
	idleTimeout time.Duration

	mu          sync.Mutex
	idle        []*pooledClient
	activeCount int
	closed      bool
}

// NewPool initializes a FastCGI connection pool.
func NewPool(cfg PoolConfig) *Pool {
	if cfg.MaxIdle <= 0 {
		cfg.MaxIdle = 10
	}
	if cfg.MaxActive <= 0 {
		cfg.MaxActive = 100
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 60 * time.Second
	}
	return &Pool{
		network:     cfg.Network,
		address:     cfg.Address,
		maxIdle:     cfg.MaxIdle,
		maxActive:   cfg.MaxActive,
		idleTimeout: cfg.IdleTimeout,
		idle:        make([]*pooledClient, 0, cfg.MaxIdle),
	}
}

// Get acquires a FastCGI client from the pool or dials a new connection.
func (p *Pool) Get(ctx context.Context) (*Client, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, ErrPoolClosed
	}

	// Clean expired idle connections
	now := time.Now()
	for len(p.idle) > 0 {
		pc := p.idle[len(p.idle)-1]
		p.idle = p.idle[:len(p.idle)-1]

		if p.idleTimeout > 0 && now.Sub(pc.lastUsed) > p.idleTimeout {
			_ = pc.client.Close()
			p.activeCount--
			continue
		}

		p.mu.Unlock()
		return pc.client, nil
	}

	if p.maxActive > 0 && p.activeCount >= p.maxActive {
		p.mu.Unlock()
		return nil, ErrPoolExhausted
	}

	p.activeCount++
	p.mu.Unlock()

	client, err := NewClient(p.network, p.address, 10*time.Second)
	if err != nil {
		p.mu.Lock()
		p.activeCount--
		p.mu.Unlock()
		return nil, fmt.Errorf("fastcgi pool dial: %w", err)
	}

	return client, nil
}

// Put returns a healthy FastCGI client back to the pool.
func (p *Pool) Put(client *Client, err error) {
	if client == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed || err != nil {
		_ = client.Close()
		p.activeCount--
		return
	}

	if len(p.idle) < p.maxIdle {
		p.idle = append(p.idle, &pooledClient{
			client:   client,
			lastUsed: time.Now(),
		})
	} else {
		_ = client.Close()
		p.activeCount--
	}
}

// Close terminates all pooled idle connections.
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true
	for _, pc := range p.idle {
		_ = pc.client.Close()
	}
	p.idle = nil
	p.activeCount = 0
	return nil
}

// Stats returns current pool metrics.
func (p *Pool) Stats() (idleCount, activeCount int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.idle), p.activeCount
}
