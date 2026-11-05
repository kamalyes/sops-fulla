/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-05 00:00:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-05 00:00:00
 * @FilePath: \sops-fulla\service\dialog.go
 * @Description: 弹出系统目录选择窗口：浏览器拿不到本机绝对路径，由本地服务代劳
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package service

import (
	"context"
	stderrors "errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/kamalyes/sops-fulla/errors"
)

// dialogWait 目录选择窗口的最长等待时间；超过则认为窗口出了问题
const dialogWait = 10 * time.Minute

// PickDirectory 弹出系统自带的目录选择窗口，返回选好的绝对路径；
// 用户直接关掉窗口时返回空字符串。浏览器出于安全限制拿不到本机
// 绝对路径，所以这一步交给本地服务来做。
func PickDirectory() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dialogWait)
	defer cancel()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// PowerShell 的窗体要求单线程套间（-STA），否则窗口弹不出来；
		// 借一个置顶的空窗口做属主，避免选择窗口被浏览器盖住
		script := `Add-Type -AssemblyName System.Windows.Forms | Out-Null
$dialog = New-Object System.Windows.Forms.FolderBrowserDialog
$dialog.Description = '选择配置仓库'
$dialog.ShowNewFolderButton = $false
$owner = New-Object System.Windows.Forms.Form
$owner.TopMost = $true
if ($dialog.ShowDialog($owner) -eq [System.Windows.Forms.DialogResult]::OK) {
  Write-Output $dialog.SelectedPath
}`
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-STA", "-Command", script)
	case "darwin":
		if _, err := exec.LookPath("osascript"); err != nil {
			return "", errors.New(501, errors.BizErrCodeBrowseUnsupported, "当前系统缺少目录选择组件，请直接输入路径")
		}
		cmd = exec.CommandContext(ctx, "osascript", "-e", `POSIX path of (choose folder with prompt "选择配置仓库")`)
	default:
		// Linux：优先 zenity，其次 kdialog，都没有则明确告知不支持
		if _, err := exec.LookPath("zenity"); err == nil {
			cmd = exec.CommandContext(ctx, "zenity", "--file-selection", "--directory", "--title=选择配置仓库")
		} else if _, err := exec.LookPath("kdialog"); err == nil {
			cmd = exec.CommandContext(ctx, "kdialog", "--getexistingdirectory", "--title", "选择配置仓库", os.Getenv("HOME"))
		} else {
			return "", errors.New(501, errors.BizErrCodeBrowseUnsupported, "当前系统缺少目录选择组件，请直接输入路径")
		}
	}

	output, err := cmd.Output()
	if ctx.Err() != nil {
		return "", errors.New(500, errors.BizErrCodeBrowseFailed, "目录选择窗口超时未响应，请重试或直接输入路径")
	}
	if err != nil {
		var exit *exec.ExitError
		if stderrors.As(err, &exit) {
			return "", nil // 用户关掉了选择窗口，不算出错
		}
		return "", errors.New(500, errors.BizErrCodeBrowseFailed, "目录选择窗口没能打开，请直接输入路径")
	}
	return strings.TrimSpace(string(output)), nil
}
