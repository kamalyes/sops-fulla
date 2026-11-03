/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 22:20:37
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 22:38:15
 * @FilePath: \sops-fulla\bootstrap\server.go
 * @Description: 管理页面子命令：启动HTTP服务、打开浏览器与优雅退出
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package bootstrap

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/kamalyes/sops-fulla/server"
	"github.com/kamalyes/sops-fulla/service"
)

// webOptions 是页面子命令的可调项
type webOptions struct {
	port        int
	noBrowser   bool
	secretsName string
	version     string
}

// runWeb 启动本地管理页面，阻塞直到收到退出信号，返回进程退出码
func runWeb(svc *service.Service, root string, args []string, options webOptions) int {
	// 兼容 scripts/secrets.sh web --port/--no-browser 的传参方式：
	// 页面自己的 flags 允许出现在子命令之后
	flags := flag.NewFlagSet("web", flag.ContinueOnError)
	port := flags.Int("port", options.port, "页面端口")
	noBrowser := flags.Bool("no-browser", options.noBrowser, "不自动打开浏览器")
	secretsName := flags.String("secrets-name", options.secretsName, "密钥文件约定名")
	flags.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	if err := flags.Parse(args); err != nil {
		return 2
	}
	svc.SetSecretsName(*secretsName)

	srv := server.New(svc)
	if err := srv.Start(fmt.Sprintf("127.0.0.1:%d", *port)); err != nil {
		fmt.Fprintln(os.Stderr, "管理页面启动失败，端口可能已被占用；可用 --port 指定其他端口")
		return 1
	}
	url := fmt.Sprintf("http://127.0.0.1:%d", srv.Port())
	fmt.Printf("配置管理页面：%s\n", url)
	fmt.Printf("配置仓库：%s\n", root)
	fmt.Printf("版本：%s\n", options.version)
	fmt.Println("提交、推送及部署仍按现有流程执行按 Ctrl+C 退出")
	if !*noBrowser {
		time.AfterFunc(300*time.Millisecond, func() { openBrowser(url) })
	}
	waitForShutdown(srv)
	return 0
}

// waitForShutdown 阻塞等待 Ctrl+C / SIGTERM，再优雅停止服务
func waitForShutdown(srv *server.Server) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shutdownContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	srv.Shutdown(shutdownContext)
}

// openBrowser 尽力打开系统浏览器；打不开也不影响服务本身
func openBrowser(url string) {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		command = exec.Command("open", url)
	default:
		command = exec.Command("xdg-open", url)
	}
	command.Start()
}
