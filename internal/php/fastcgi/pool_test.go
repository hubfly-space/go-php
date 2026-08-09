package fastcgi

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestPool_GetAndPut(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer l.Close()

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	pool := NewPool(PoolConfig{
		Network:     "tcp",
		Address:     l.Addr().String(),
		MaxIdle:     2,
		MaxActive:   5,
		IdleTimeout: 1 * time.Second,
	})
	defer pool.Close()

	client1, err := pool.Get(context.Background())
	if err != nil {
		t.Fatalf("pool.Get failed: %v", err)
	}

	idle, active := pool.Stats()
	if active != 1 {
		t.Errorf("active = %d, want 1", active)
	}
	if idle != 0 {
		t.Errorf("idle = %d, want 0", idle)
	}

	pool.Put(client1, nil)
	idle, active = pool.Stats()
	if idle != 1 || active != 1 {
		t.Errorf("after Put: idle=%d active=%d, want 1, 1", idle, active)
	}

	// Next Get should reuse client1
	client2, err := pool.Get(context.Background())
	if err != nil {
		t.Fatalf("pool.Get reuse failed: %v", err)
	}
	if client1 != client2 {
		t.Errorf("expected reused client pointer match")
	}

	pool.Put(client2, nil)
}

func TestPool_Close(t *testing.T) {
	pool := NewPool(PoolConfig{
		Network: "tcp",
		Address: "127.0.0.1:0",
	})
	_ = pool.Close()

	_, err := pool.Get(context.Background())
	if err != ErrPoolClosed {
		t.Errorf("got err %v, want ErrPoolClosed", err)
	}
}
