package main

import (
	"io"
	"log"
	"net"

	pty "github.com/threatexpert/go-winpty"
)

func handleConn(conn net.Conn) {
	defer conn.Close()

	// 每个连接启动一个新的 pty + cmd.exe
	pt, err := pty.New()
	if err != nil {
		log.Printf("[%s] failed to open pty: %v", conn.RemoteAddr(), err)
		return
	}

	cmd := pt.Command("cmd.exe")
	if err := cmd.Start(); err != nil {
		log.Printf("[%s] failed to start cmd.exe: %v", conn.RemoteAddr(), err)
		pt.Close()
		return
	}

	log.Printf("[%s] started cmd.exe (PID=%d)", conn.RemoteAddr(), cmd.Process.Pid)

	// 双向转发：客户端 ↔ cmd.exe
	done := make(chan struct{}, 2)

	go func() {
		defer func() { done <- struct{}{} }()
		_, err := io.Copy(pt, conn) // 客户端输入 → cmd
		if err != nil {
			log.Printf("[%s] copy conn->pty error: %v", conn.RemoteAddr(), err)
		}
	}()

	go func() {
		defer func() { done <- struct{}{} }()
		_, err := io.Copy(conn, pt) // cmd 输出 → 客户端
		type closeWriter interface {
			CloseWrite() error
		}
		if cw, ok := conn.(closeWriter); ok {
			cw.CloseWrite()
		}
		if err != nil {
			log.Printf("[%s] copy pty->conn error: %v", conn.RemoteAddr(), err)
		}
	}()

	go func() {
		defer func() { done <- struct{}{} }()
		cmd.Wait()
	}()

	// 等待任一方向结束
	<-done
	conn.Close()
	pt.Close()

	// 杀掉 cmd.exe
	_ = cmd.Process.Kill()
	_ = cmd.Wait()

	log.Printf("[%s] session closed", conn.RemoteAddr())
}

func main() {
	addr := ":2222"
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", addr, err)
	}
	log.Printf("listening on %s ...", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}
		log.Printf("new connection from %s", conn.RemoteAddr())

		go handleConn(conn)
	}
}
