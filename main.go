package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net"
	"net/http"
	"net/url"
	"order_processor/internal/adapters/miaoshou/web"
	"order_processor/internal/adapters/miaoshou/web/browser"
	"order_processor/internal/server"
	"order_processor/internal/store"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	dir := env("ORDER_DATA_DIR", filepath.Join(root, "data"))
	dir, err = filepath.Abs(dir)
	if err != nil {
		log.Fatal(err)
	}
	mode := env("ORDER_BROWSER_MODE", "docker")
	publicOrigin := strings.TrimRight(os.Getenv("ORDER_PUBLIC_ORIGIN"), "/")
	if mode != "local" && mode != "docker" {
		log.Fatal("ORDER_BROWSER_MODE 必须为 local 或 docker")
	}
	if publicOrigin != "" {
		u, e := url.Parse(publicOrigin)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			log.Fatal("ORDER_PUBLIC_ORIGIN 格式不正确")
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var runtime browser.Runtime
	var worker *exec.Cmd
	if err = os.MkdirAll(dir, 0700); err != nil {
		log.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dir, "orders.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if mode == "docker" {
		remote := &browser.Docker{DataDir: dir, Image: env("ORDER_BROWSER_IMAGE", "order-processor-browser:1.63.0"), Prefix: env("ORDER_BROWSER_PREFIX", "op-browser-")}
		checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		checkErr := remote.Check(checkCtx)
		cancel()
		if checkErr != nil {
			log.Fatal(checkErr)
		}
		runtime = remote
	} else {
		secret := make([]byte, 32)
		if _, err = rand.Read(secret); err != nil {
			log.Fatal(err)
		}
		token := hex.EncodeToString(secret)
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			log.Fatal(err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		listener.Close()
		worker = exec.CommandContext(ctx, "node", "internal/adapters/miaoshou/web/worker/worker.mjs")
		worker.Env = append(os.Environ(), server.WorkerEnv(port, token, dir)...)
		worker.Cancel = func() error { return worker.Process.Signal(syscall.SIGTERM) }
		worker.WaitDelay = 8 * time.Second
		worker.Stdout = os.Stdout
		worker.Stderr = os.Stderr
		if err = worker.Start(); err != nil {
			log.Fatal(err)
		}
		runtime = browser.Local{Port: port, Token: token}
	}
	if n, e := store.ReplayArchives(db, dir, web.Decoder{}); e != nil {
		log.Printf("本地响应解析失败: %v", e)
	} else {
		log.Printf("已解析 %d 份本地响应", n)
	}
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, e := store.ReplayArchives(db, dir, web.Decoder{}); e != nil {
					log.Printf("本地响应解析失败: %v", e)
				}
			}
		}
	}()
	api := server.NewWithRuntimeContext(ctx, db, dir, runtime, publicOrigin)
	if worker != nil {
		go func() {
			if e := worker.Wait(); e != nil && ctx.Err() == nil {
				log.Printf("浏览器服务退出: %v", e)
				stop()
			}
		}()
	}
	srv := &http.Server{Addr: env("ORDER_LISTEN", "0.0.0.0:17381"), Handler: api, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(c)
	}()
	log.Printf("工作台监听 %s，浏览器模式 %s（先运行 npm run build）", srv.Addr, mode)
	if err = srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Print(err)
		stop()
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
