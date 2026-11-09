package main

import (
	"context"
	"errors"
	"sync/atomic"

	"google.golang.org/grpc"
)

// rpcConn is the subset of *grpc.ClientConn the generated clients use. Keeping
// it narrow makes the pool testable without a live server.
type rpcConn interface {
	grpc.ClientConnInterface
	Close() error
}

// connPool round-robins calls over several connections to the same service. One
// HTTP/2 connection multiplexes many streams, but a single TCP connection
// becomes the bottleneck around 15k QPS and is a single point of failure during
// a rolling restart. A handful of connections spreads both the streams and the
// reconnect cost.
type connPool struct {
	conns []rpcConn
	next  atomic.Uint64
}

func newConnPool(conns ...rpcConn) *connPool {
	return &connPool{conns: conns}
}

// pick returns the next connection. The counter is monotonic and the modulo is
// applied on read, so concurrent callers never contend on a lock.
func (p *connPool) pick() rpcConn {
	return p.conns[p.next.Add(1)%uint64(len(p.conns))]
}

func (p *connPool) Invoke(ctx context.Context, method string, args, reply any, opts ...grpc.CallOption) error {
	return p.pick().Invoke(ctx, method, args, reply, opts...)
}

func (p *connPool) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return p.pick().NewStream(ctx, desc, method, opts...)
}

func (p *connPool) Close() error {
	var err error
	for _, conn := range p.conns {
		err = errors.Join(err, conn.Close())
	}
	return err
}
