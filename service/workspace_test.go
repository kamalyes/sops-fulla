/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-05 07:00:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-05 10:56:00
 * @FilePath: \sops-fulla\service\workspace_test.go
 * @Description: 多工作区与记住私钥的端到端测试（FULLA_CONFIG_DIR 隔离本机配置）
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package service_test

import (
	"path/filepath"
	"testing"

	"filippo.io/age"

	"github.com/kamalyes/sops-fulla/models"
	"github.com/kamalyes/sops-fulla/service"
)

// isolateConfig 把工具配置目录指到临时目录，测试不碰本机真实配置
func isolateConfig(t *testing.T) {
	t.Helper()
	t.Setenv("FULLA_CONFIG_DIR", t.TempDir())
	t.Setenv("SOPS_AGE_KEY", "")
	t.Setenv("SOPS_AGE_KEY_FILE", "")
}

// newRepo 建一个只含 .sops.yaml 与一份明文配置的仓库，返回绝对路径
func newRepo(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	writeFile(t, filepath.Join(root, ".sops.yaml"), "creation_rules:\n"+
		"  - path_regex: \\.enc\\.(yaml|yml|json)$\n"+
		"    age: age1kxev2e0g0r6a4km5x5rmadrfrkqgv0navtnn5xy4fdsa5cj56q7qj6pfq5\n")
	writeFile(t, filepath.Join(root, "demo", "dev", "config.dev.yaml"), "Name: demo\n")
	return root
}

// TestEmptyWorkspaceStartsAndSwitches 验证免仓库启动、空工作区报错与切换
func TestEmptyWorkspaceStartsAndSwitches(t *testing.T) {
	isolateConfig(t)
	svc := service.NewIsolated("")

	catalog := svc.Catalog()
	if catalog.HasWorkspace {
		t.Fatal("空启动时 has_workspace 应为 false")
	}
	if len(catalog.Services) != 0 {
		t.Fatalf("空工作区不应有服务，实际 %+v", catalog.Services)
	}
	if catalog.Repository != "" {
		t.Fatalf("空工作区的仓库名应为空，实际 %q", catalog.Repository)
	}

	// 空工作区下读文件统一报 400
	if _, err := svc.Read("", "", "config.yaml"); err == nil {
		t.Fatal("空工作区读取应报错")
	} else {
		mustFail(t, err, 400, "请先选择配置仓库")
	}
	content := "Name: demo\n"
	if _, err := svc.Save(&models.SaveRequest{Service: "demo", Environment: "dev", Filename: "config.dev.yaml", Content: &content}); err == nil {
		t.Fatal("空工作区保存应报错")
	} else {
		mustFail(t, err, 400, "请先选择配置仓库")
	}

	// 缺 .sops.yaml 的目录不能当工作区
	mustFail(t, svc.SwitchRoot(t.TempDir()), 400, ".sops.yaml")
	mustFail(t, svc.SwitchRoot("   "), 400, "请填写配置仓库路径")

	// 切换到有效仓库后目录可用，且进入最近列表
	root := newRepo(t, "alpha")
	if err := svc.SwitchRoot(root); err != nil {
		t.Fatal(err)
	}
	catalog = svc.Catalog()
	if !catalog.HasWorkspace || catalog.Repository != "alpha" || len(catalog.Services) != 1 {
		t.Fatalf("切换后能力信息不正确：%+v", catalog.Capabilities)
	}
	if _, err := svc.Read("demo", "dev", "config.dev.yaml"); err != nil {
		t.Fatalf("切换后应能读取配置：%v", err)
	}

	// 再切换一个仓库：最近列表按最近使用排序
	beta := newRepo(t, "beta")
	if err := svc.SwitchRoot(beta); err != nil {
		t.Fatal(err)
	}
	info := svc.WorkspacesInfo()
	if info.Current == "" || filepath.Base(info.Current) != "beta" {
		t.Fatalf("当前工作区应是 beta，实际 %q", info.Current)
	}
	if len(info.Recent) != 2 || filepath.Base(info.Recent[0]) != "beta" || filepath.Base(info.Recent[1]) != "alpha" {
		t.Fatalf("最近列表应按使用顺序排列，实际 %+v", info.Recent)
	}

	// 切换不改变密钥文件约定名
	svc.SetSecretsName("credentials.enc.yaml")
	if err := svc.SwitchRoot(root); err != nil {
		t.Fatal(err)
	}
	data, err := svc.Read("demo", "dev", "credentials.enc.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if data.Kind != "secrets" {
		t.Fatalf("约定名应跟随切换：credentials.enc.yaml 应按密钥文件对待，实际 %q", data.Kind)
	}
}

// TestRememberKeyRoundTrip 验证记住的私钥：写入、下次启动自动加载、清除
func TestRememberKeyRoundTrip(t *testing.T) {
	isolateConfig(t)
	root := newRepo(t, "alpha")
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}

	svc := service.NewIsolated(root)
	capabilities := svc.Capabilities()
	if capabilities.SecretsReady || capabilities.KeySource != "" {
		t.Fatalf("初始不应有可用私钥，实际 %+v", capabilities)
	}

	// 无效私钥直接拒绝，不落盘
	if _, err := svc.RememberKey("not-a-key"); err == nil {
		t.Fatal("无效私钥应报错")
	} else {
		mustFail(t, err, 400, "age 私钥")
	}

	// 记住后本实例立即可用，来源标记为 remembered
	capabilities, err = svc.RememberKey(identity.String())
	if err != nil {
		t.Fatal(err)
	}
	if !capabilities.SecretsReady || capabilities.KeySource != "remembered" {
		t.Fatalf("记住后应立即可用（remembered），实际 %+v", capabilities)
	}

	// 模拟下次启动：新实例自动从记住的文件加载
	next := service.New(root)
	capabilities = next.Capabilities()
	if !capabilities.SecretsReady || capabilities.KeySource != "remembered" {
		t.Fatalf("下次启动应自动加载记住的私钥，实际 %+v", capabilities)
	}

	// 清除后回落：不再来自 remembered
	capabilities, err = next.ForgetRememberedKey()
	if err != nil {
		t.Fatal(err)
	}
	if capabilities.KeySource == "remembered" {
		t.Fatalf("清除后不应再使用 remembered，实际 %+v", capabilities)
	}
	again := service.New(root)
	if again.Capabilities().KeySource == "remembered" {
		t.Fatal("清除后的新实例不应再加载记住的私钥")
	}
}

// TestAppearancePersistence 验证个性化设置写入本机配置目录并在下次读取时还原
func TestAppearancePersistence(t *testing.T) {
	isolateConfig(t)

	stored := service.Appearance{Accent: "#7c5cd6", Sans: "kai", Mono: "consolas", Zoom: "relaxed"}
	if err := service.SaveAppearance(stored); err != nil {
		t.Fatal(err)
	}
	loaded := service.LoadAppearance()
	if loaded != stored {
		t.Fatalf("持久化后应原样读回，实际 %+v", loaded)
	}

	// 非法取值被归零成默认：主题色必须是 #RRGGBB，字体/字号必须在白名单里
	if err := service.SaveAppearance(service.Appearance{Accent: "red", Sans: "comic-sans", Mono: "hacker", Zoom: "huge"}); err == nil {
		t.Fatal("全非法取值应报错")
	}
	if loaded := service.LoadAppearance(); loaded != stored {
		t.Fatalf("被拒绝的保存不应改动已存内容，实际 %+v", loaded)
	}

	// 无文件时返回默认值
	isolateConfig(t)
	if loaded := service.LoadAppearance(); loaded.Sans != "default" || loaded.Zoom != "standard" {
		t.Fatalf("无文件时应返回默认值，实际 %+v", loaded)
	}
}
