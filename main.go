package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"order_processor/internal/adapters/miaoshou/web"
	"order_processor/internal/adapters/miaoshou/web/browser"
	"order_processor/internal/server"
	"order_processor/internal/store"
	"os"
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
	publicOrigin := strings.TrimRight(os.Getenv("ORDER_PUBLIC_ORIGIN"), "/")
	if publicOrigin != "" {
		u, e := url.Parse(publicOrigin)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
			log.Fatal("ORDER_PUBLIC_ORIGIN 格式不正确")
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err = os.MkdirAll(dir, 0700); err != nil {
		log.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dir, "orders.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	runtime := &browser.Docker{DataDir: dir, Image: env("ORDER_BROWSER_IMAGE", "order-processor-browser:1.63.0"), Prefix: env("ORDER_BROWSER_PREFIX", "op-browser-")}
	checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	checkErr := runtime.Check(checkCtx)
	cancel()
	if checkErr != nil {
		log.Fatal(checkErr)
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
	srv := &http.Server{Addr: env("ORDER_LISTEN", "0.0.0.0:17381"), Handler: api, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(c)
	}()
	log.Printf("工作台监听 %s，使用容器浏览器（先运行 npm run build）", srv.Addr)
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
