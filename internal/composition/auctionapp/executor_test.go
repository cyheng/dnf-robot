package auctionapp

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"robot/internal/capability/marketapp"
)

func TestCollectDisconnectDoesNotReconnectOrResend(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var connections atomic.Int32
	var packets atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			connections.Add(1)
			_ = conn.SetReadDeadline(time.Now().Add(time.Second))
			packet := make([]byte, 0x33)
			if _, err := io.ReadFull(conn, packet); err == nil && packet[1] == 5 && binary.LittleEndian.Uint32(packet[0x27:0x2b]) == 123 {
				packets.Add(1)
			}
			conn.Close()
		}
	}()
	cfg := marketapp.DefaultConfig()
	cfg.AuctionHost = "127.0.0.1"
	cfg.AuctionPort = listener.Addr().(*net.TCPAddr).Port
	executor := NewFactory().NewActionExecutor(cfg)
	defer executor.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = executor.Execute(ctx, marketapp.Action{Market: "auction", Operation: "collect", OwnerID: 90100001, AuctionID: 1, InstantPrice: 123})
	if err == nil {
		t.Fatal("断线被标记为成功")
	}
	listener.Close()
	<-done
	if got := connections.Load(); got != 1 {
		t.Fatalf("购买断线后建立连接 %d 次", got)
	}
	if got := packets.Load(); got != 1 {
		t.Fatalf("玩家挂价购买请求发送次数=%d", got)
	}
}
