// 航标（hangbiao）：把「有序策略组」模型搬到 Web 与 Docker 里。
// 模型参考 pengchujin/tower（MIT），本项目为独立实现，界面与服务端均为重写。
package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"towerdocker/internal/api"
	"towerdocker/internal/scheme"
	"towerdocker/internal/store"
	"towerdocker/internal/sub"
)

//go:embed all:web/dist
var distFS embed.FS

func main() {
	addr := flag.String("addr", ":8630", "监听地址")
	dataDir := flag.String("data", "./data", "数据目录")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("数据目录不可用：%v", err)
	}
	st, err := store.Open(filepath.Join(*dataDir, "state.json"))
	if err != nil {
		log.Fatalf("读取数据失败：%v", err)
	}
	// 运行时可从 ACL4SSR 上游同步最新的 ini；同步结果缓存在数据目录里。
	// 没同步过就没有任何方案与分组，避免把未经同步的分组导出给客户端。
	scheme.SetCachePath(filepath.Join(*dataDir, "acl4ssr-sync.json"))
	if err := scheme.LoadCache(); err != nil {
		log.Printf("读取 ACL4SSR 同步缓存失败（请先在界面同步 ACL4SSR）：%v", err)
	}

	static, err := fs.Sub(distFS, "web/dist")
	if err != nil {
		log.Fatalf("前端资源缺失：%v", err)
	}

	engine := api.New(st, static)
	go engine.AutoRefresh()
	// 启动后探测 GitHub raw 加速源，并每 30 分钟复核一次；导出时按结果改写规则集地址。
	go func() {
		for {
			sub.Detect(context.Background())
			time.Sleep(30 * time.Minute)
		}
	}()
	srv := &http.Server{
		Addr:              *addr,
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("航标已启动 %s，数据目录 %s，订阅地址 /sub?target=auto", *addr, *dataDir)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
