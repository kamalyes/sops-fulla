/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 20:31:08
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 21:58:26
 * @FilePath: \sops-fulla\bootstrap\app.go
 * @Description: 工具入口装配：flags解析、仓库校验与子命令分发
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

// Package bootstrap 是工具的入口装配层：解析命令行、校验仓库，
// 再把控制权交给管理页面或密文批量子命令
package bootstrap

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kamalyes/sops-fulla/batch"
	"github.com/kamalyes/sops-fulla/constants"
	"github.com/kamalyes/sops-fulla/service"
)

const usage = `sops-fulla —— 通用的本地配置仓库管理面板

用法：
  sops-fulla [flags]              启动本地管理页面（默认）
  sops-fulla [flags] <子命令>      批量管理仓库里的 *.enc.* 密文

配置仓库约定（可通过 flags 适配）：
  <任意目录层级>/*.yaml|*.yml|*.json     明文配置与密钥文件（位置不限）
  .sops.yaml                           age 公钥与「文件名带 .enc. 才加密」的规则
  密钥文件默认叫 secrets.enc.yaml       扁平 KEY: value；名字不同用 --secrets-name 适配

从零开始不用装 sops：init 会生成 age 密钥并写好 .sops.yaml；
加解密在进程内原生完成解密需要 age 私钥：
页面在「私钥」弹窗里填写（优先），或配置 SOPS_AGE_KEY / SOPS_AGE_KEY_FILE，
或把私钥放在 ~/.config/sops/age/keys.txt

flags：
  --repo <路径>            配置仓库路径，默认当前目录
  --port <端口>            页面端口，默认 8765
  --no-browser             不自动打开浏览器
  --secrets-name <文件名>   密钥文件约定名，默认 secrets.enc.yaml
  --version                打印版本

子命令：
  init [--example] 初始化新仓库：生成 age 密钥、写 .sops.yaml、可选示例结构
  status          每份是密文还是明文、各几个键（不打印值）
  keys            只列键名（不打印值）
  encrypt         把所有明文的就地加密，逐份验证解回来一致
  decrypt --yes   就地解开全部（改完记得 encrypt 回去）
  show [路径]      解出来打到标准输出，不落盘
  edit <路径>      命令行交互编辑，保存时重新加密
  check           有明文就退 1（给 CI 或 pre-commit 用）
`

// Run 是工具入口：解析 flags、校验仓库、分发子命令，返回进程退出码
func Run(args []string, version string) int {
	flags := flag.NewFlagSet(constants.ServiceName, flag.ContinueOnError)
	repo := flags.String("repo", ".", "配置仓库路径，默认当前目录")
	port := flags.Int("port", constants.DefaultPort, "页面端口")
	noBrowser := flags.Bool("no-browser", false, "不自动打开浏览器")
	secretsName := flags.String("secrets-name", "", "密钥文件约定名，默认 secrets.enc.yaml")
	showVersion := flags.Bool("version", false, "打印版本")
	flags.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Println(constants.ServiceName + " " + version)
		return 0
	}
	args = flags.Args()
	command := "web"
	if len(args) > 0 {
		command, args = args[0], args[1:]
	}

	root, err := filepath.Abs(*repo)
	if err != nil {
		fmt.Fprintln(os.Stderr, "请通过 --repo 指定配置仓库路径，该目录应包含 .sops.yaml")
		return 1
	}
	// init 是唯一不需要 .sops.yaml 的子命令：它就是用来创建这份文件的
	if command == "init" {
		return RunInit(root, args, os.Stdout, os.Stderr)
	}
	if !hasSopsRules(root) {
		fmt.Fprintln(os.Stderr, "请通过 --repo 指定配置仓库路径，该目录应包含 .sops.yaml")
		return 1
	}
	svc := service.New(root)
	svc.SetSecretsName(*secretsName)
	switch command {
	case "web":
		return runWeb(svc, root, args, webOptions{port: *port, noBrowser: *noBrowser, secretsName: *secretsName, version: version})
	case "status", "keys", "check", "encrypt", "decrypt", "show", "edit":
		return batch.RunBatch(svc, command, args, os.Stdout, os.Stderr)
	default:
		fmt.Fprintf(os.Stderr, "不认识的子命令：%s\n%s", command, usage)
		return 1
	}
}

// hasSopsRules 判断目录里是否有 .sops.yaml 加密规则文件
func hasSopsRules(root string) bool {
	info, err := os.Stat(filepath.Join(root, ".sops.yaml"))
	return err == nil && !info.IsDir()
}
