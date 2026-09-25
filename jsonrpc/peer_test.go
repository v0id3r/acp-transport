package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestPeerHandlesNotificationsInReceiveOrder(t *testing.T) {
	server, client := Pipe(8)
	defer server.Close()
	defer client.Close()

	var mu sync.Mutex
	var got []int
	done := make(chan struct{})

	peer := NewPeer(server, HandlerFunc(func(_ context.Context, req Request) (json.RawMessage, *Error) {
		var params struct {
			N int `json:"n"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			t.Errorf("decode params: %v", err)
			return nil, nil
		}
		if params.N == 1 {
			time.Sleep(50 * time.Millisecond)
		}
		mu.Lock()
		got = append(got, params.N)
		if len(got) == 2 {
			close(done)
		}
		mu.Unlock()
		return nil, nil
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = peer.Serve(ctx) }()

	first, err := NewNotification("note", map[string]int{"n": 1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewNotification("note", map[string]int{"n": 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Send(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := client.Send(ctx, second); err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for notifications")
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatalf("notification order = %v, want [1 2]", got)
	}
}

func TestPeerNilHandlerResultReturnsNull(t *testing.T) {
	server, client := Pipe(8)
	defer server.Close()
	defer client.Close()

	peer := NewPeer(server, HandlerFunc(func(context.Context, Request) (json.RawMessage, *Error) {
		return nil, nil
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = peer.Serve(ctx) }()
	clientPeer := NewPeer(client, nil)
	go func() { _ = clientPeer.Serve(ctx) }()

	got, err := clientPeer.Call(ctx, "empty", nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "null" {
		t.Fatalf("result = %s, want null", got)
	}
}

func TestPeerCallReturnsBeforeCancelNotificationSendCompletes(t *testing.T) {
	conn := &blockingCancelConn{
		requestSent:   make(chan struct{}),
		cancelStarted: make(chan struct{}),
		releaseCancel: make(chan struct{}),
	}
	peer := NewPeer(conn, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := peer.Call(ctx, "slow", nil)
		done <- err
	}()

	<-conn.requestSent
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Call error = %v, want context.Canceled", err)
		}
	case <-time.After(100 * time.Millisecond):
		close(conn.releaseCancel)
		t.Fatal("Call blocked on best-effort cancel notification")
	}

	select {
	case <-conn.cancelStarted:
	case <-time.After(time.Second):
		t.Fatal("cancel notification was not attempted")
	}
	close(conn.releaseCancel)
}

type blockingCancelConn struct {
	requestSent   chan struct{}
	cancelStarted chan struct{}
	releaseCancel chan struct{}
	onceRequest   sync.Once
	onceCancel    sync.Once
}

func (c *blockingCancelConn) Send(ctx context.Context, msg *Message) error {
	if msg.Method == "$/cancel_request" {
		c.onceCancel.Do(func() { close(c.cancelStarted) })
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.releaseCancel:
			return nil
		}
	}
	c.onceRequest.Do(func() { close(c.requestSent) })
	return nil
}

func (*blockingCancelConn) Receive(ctx context.Context) (*Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (*blockingCancelConn) Close() error {
	return io.EOF
}

func TestPeerCancelRequestCancelsInboundHandler(t *testing.T) {
	server, client := Pipe(8)
	defer server.Close()
	defer client.Close()

	peer := NewPeer(server, HandlerFunc(func(ctx context.Context, _ Request) (json.RawMessage, *Error) {
		<-ctx.Done()
		return json.RawMessage(`{"outcome":"cancelled"}`), nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = peer.Serve(ctx) }()

	req, err := NewRequest(json.RawMessage(`7`), "session/request_permission", nil)
	if err != nil {
		t.Fatal(err)
	}
	note, err := NewNotification(cancelRequestMethod, map[string]int{"requestId": 7})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Send(ctx, req); err != nil {
		t.Fatal(err)
	}
	if err := client.Send(ctx, note); err != nil {
		t.Fatal(err)
	}

	recvCtx, recvCancel := context.WithTimeout(ctx, time.Second)
	defer recvCancel()
	resp, err := client.Receive(recvCtx)
	if err != nil {
		t.Fatalf("waiting for cancelled response: %v", err)
	}
	if string(*resp.ID) != "7" || string(resp.Result) != `{"outcome":"cancelled"}` {
		t.Fatalf("response = id %s result %s", *resp.ID, resp.Result)
	}
}
