//go:build windows

// twloader-tool/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"twloader-tool/api"
	"twloader-tool/config"
	"twloader-tool/game"
	"twloader-tool/optimizer"
	"twloader-tool/ui"
	"twloader-tool/utils"

	"github.com/faiface/mainthread"
)

const serverAddr = "127.0.0.1:8787"

var logger = log.New(os.Stdout, "TWLOADERWEB | ", log.LstdFlags)

func runApp() {
	// 1. 初始化
	if err := config.Load(); err != nil {
		logger.Printf("警告：讀取設定檔時發生錯誤: %v", err)
	}
	if err := optimizer.FetchItemsFromServer(); err != nil {
		logger.Fatalf("初始化失敗，無法獲取優化項目列表: %v", err)
	}
	if err := api.FetchStaticAssets(); err != nil {
		logger.Fatalf("初始化失敗，無法獲取前端介面檔案: %v", err)
	}

	// 2. 啟動背景任務
	go game.CheckVersion()
	game.SetupGamePathLink()

	// 3. 啟動 GUI 管理器
	guiReadyChan := make(chan bool)
	go ui.GUIManager(guiReadyChan)
	<-guiReadyChan
	logger.Println("GUI 管理器已就緒。")

	// 4. 設定 HTTP 伺服器和路由
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	server := &http.Server{
		Addr:    serverAddr,
		Handler: mux,
	}

	// 5. 啟動伺服器並開啟瀏覽器
	go func() {
		logger.Printf("伺服器正在監聽 http://%s\n", serverAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("無法啟動伺服器: %v", err)
		}
	}()
	time.Sleep(500 * time.Millisecond)
	utils.OpenBrowser(fmt.Sprintf("http://%s", serverAddr))

	// 6. 等待關閉信號
	<-api.ShutdownChan
	logger.Println("偵測到前端關閉，正在關閉 HTTP 伺服器...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		logger.Printf("伺服器關閉時發生錯誤: %v", err)
	}

	ui.CloseGUIManager()
	logger.Println("runApp 函式結束。")
}

func main() {
	mainthread.Run(runApp)
	logger.Println("程式已完全關閉。")
}
