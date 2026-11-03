/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 20:17:52
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 21:57:11
 * @FilePath: \sops-fulla\batch\batch.go
 * @Description: 密文批量管理子命令：status/keys/check/encrypt/decrypt/show/edit
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

// Package batch 实现密文批量管理子命令，输出形状与旧 bash 版保持一致，
// README 里的示例不用改
package batch

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/kamalyes/sops-fulla/crypto"
	"github.com/kamalyes/sops-fulla/models"
	"github.com/kamalyes/sops-fulla/repository"
	"github.com/kamalyes/sops-fulla/service"
)

// RunBatch 执行密文批量管理子命令，返回进程退出码
func RunBatch(svc *service.Service, command string, args []string, stdout, stderr io.Writer) int {
	switch command {
	case "status":
		return batchStatus(svc, stdout, stderr)
	case "keys":
		return batchKeys(svc, stdout, stderr)
	case "check":
		return batchCheck(svc, stdout, stderr)
	case "encrypt":
		return batchEncrypt(svc, stdout, stderr)
	case "decrypt":
		if len(args) == 0 || args[0] != "--yes" {
			fmt.Fprint(stderr, decryptWarning)
			return 1
		}
		return batchDecrypt(svc, stdout, stderr)
	case "show":
		return batchShow(svc, stdout, stderr, args)
	case "edit":
		if len(args) == 0 || args[0] == "" {
			fmt.Fprintln(stderr, "用法：config-manager edit <文件>")
			return 1
		}
		if err := batchEdit(svc, args[0]); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(stderr, "不认识的子命令：%s\n", command)
	return 1
}

const decryptWarning = `这会把所有密文就地解成明文，磁盘上留下真凭据
确认再来一次：config-manager decrypt --yes
只是想看内容的话用 show，它不落盘：config-manager show
`

// encryptedFiles 列出仓库里所有 *.enc.* 文件（跳过 .git），按路径排序
func encryptedFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.Contains(entry.Name(), ".enc.") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(relative))
		return nil
	})
	sort.Strings(files)
	return files, err
}

// fileReport 描述一份 *.enc.* 文件的当前形态
type fileReport struct {
	path      string
	encrypted bool
	keys      []string
	readable  bool
}

// scan 扫描仓库里所有 *.enc.* 文件，报告各自的形态与顶层键
func scan(svc *service.Service) ([]fileReport, error) {
	files, err := encryptedFiles(svc.Root())
	if err != nil {
		return nil, err
	}
	reports := make([]fileReport, 0, len(files))
	for _, file := range files {
		report := fileReport{path: file}
		if data, err := os.ReadFile(filepath.Join(svc.Root(), filepath.FromSlash(file))); err == nil {
			report.readable = true
			if document, err := models.ParseDocument(string(data), models.FileKind(file)); err == nil {
				report.encrypted = models.HasTopKey(document, "sops")
				for _, entry := range document.Map {
					if entry.Key != "sops" {
						report.keys = append(report.keys, entry.Key)
					}
				}
			}
		}
		reports = append(reports, report)
	}
	return reports, nil
}

func batchStatus(svc *service.Service, stdout, stderr io.Writer) int {
	reports, err := scan(svc)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if len(reports) == 0 {
		fmt.Fprintln(stdout, "（没有 *.enc.* 文件）")
		return 0
	}
	fmt.Fprintf(stdout, "%-52s  %-9s %s\n", "FILE", "STATE", "KEYS")
	bad := 0
	for _, report := range reports {
		if !report.readable {
			fmt.Fprintf(stdout, "%-52s  %-9s %s   <== 读不到\n", report.path, "ERROR", "-")
			bad++
			continue
		}
		if report.encrypted {
			fmt.Fprintf(stdout, "%-52s  %-9s %d\n", report.path, "encrypted", len(report.keys))
		} else {
			fmt.Fprintf(stdout, "%-52s  %-9s %d   <== 明文\n", report.path, "PLAINTEXT", len(report.keys))
			bad++
		}
	}
	fmt.Fprintln(stdout)
	if bad > 0 {
		fmt.Fprintf(stdout, "%d / %d 份是明文 —— 跑 config-manager encrypt 加密回去\n", bad, len(reports))
	} else {
		fmt.Fprintf(stdout, "%d 份全是密文\n", len(reports))
	}
	// status 只负责展示，一律退 0；要当闸门用 check，它有明文就退 1
	return 0
}

func batchKeys(svc *service.Service, stdout, stderr io.Writer) int {
	reports, err := scan(svc)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	for _, report := range reports {
		fmt.Fprintf(stdout, "── %s\n", report.path)
		for _, key := range report.keys {
			fmt.Fprintf(stdout, "   %s\n", key)
		}
	}
	return 0
}

func batchCheck(svc *service.Service, stdout, stderr io.Writer) int {
	reports, err := scan(svc)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	var bad []string
	for _, report := range reports {
		if !report.readable || !report.encrypted {
			bad = append(bad, report.path)
		}
	}
	if len(bad) > 0 {
		fmt.Fprintln(stderr, "❌ 这些名字带 .enc. 却是明文（或读不到）：")
		for _, path := range bad {
			fmt.Fprintf(stderr, "   %s\n", path)
		}
		fmt.Fprintln(stderr, "   跑 config-manager encrypt")
		return 1
	}
	fmt.Fprintln(stdout, "✅ 所有 *.enc.* 都是密文")
	return 0
}

func batchEncrypt(svc *service.Service, stdout, stderr io.Writer) int {
	reports, err := scan(svc)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	// 加密只要公钥（已在 .sops.yaml 里），但「解回来核对」需要私钥
	verify := svc.Capabilities().SecretsReady
	if !verify {
		fmt.Fprintln(stdout, "⚠️  本机没有可用私钥，加密照样能做（只要公钥），但跳过「解回来核对」这一步")
		fmt.Fprintln(stdout)
	}
	done, skip, failure := 0, 0, 0
	for _, report := range reports {
		if !report.readable {
			fmt.Fprintf(stderr, "   ❌ %s 读不到\n", report.path)
			failure++
			continue
		}
		if report.encrypted {
			fmt.Fprintf(stdout, "   跳过 %s（已是密文）\n", report.path)
			skip++
			continue
		}
		data, err := os.ReadFile(filepath.Join(svc.Root(), filepath.FromSlash(report.path)))
		if err != nil {
			fmt.Fprintf(stderr, "   ❌ %s 读不到\n", report.path)
			failure++
			continue
		}
		output, err := crypto.Encrypt(svc.Root(), data, models.FileKind(report.path), report.path)
		if err != nil {
			fmt.Fprintf(stderr, "   ❌ %s 加密失败：%s\n", report.path, err)
			failure++
			continue
		}
		if !roundTripOK(svc, data, output, report.path, stdout, stderr) {
			failure++
			continue
		}
		if err := repository.AtomicWrite(filepath.Join(svc.Root(), filepath.FromSlash(report.path)), output); err != nil {
			fmt.Fprintf(stderr, "   ❌ %s 写入失败：%s\n", report.path, err)
			failure++
			continue
		}
		done++
	}
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "共 %d 份：加密 %d，已是密文 %d，失败 %d\n", len(reports), done, skip, failure)
	return boolToInt(failure != 0)
}

// roundTripOK 解回来核对密文与加密前一致（需要私钥），并输出结果
func roundTripOK(svc *service.Service, plain, output []byte, path string, stdout, stderr io.Writer) bool {
	kind := models.FileKind(path)
	if !svc.Capabilities().SecretsReady {
		fmt.Fprintf(stdout, "   ✅ %s（%d 个键，未核对）\n", path, topKeyCount(plain, kind))
		return true
	}
	checked, err := crypto.Decrypt(output, kind, svc.Identities())
	if err != nil {
		fmt.Fprintf(stderr, "   ❌ %s 解回来失败，别提交，先人工核对\n", path)
		return false
	}
	original, _ := models.ParseDocument(string(plain), kind)
	decrypted, err := models.ParseDocument(string(checked), kind)
	if err != nil || !original.Equal(decrypted) {
		fmt.Fprintf(stderr, "   ❌ %s 解回来与加密前不一致，别提交，先人工核对\n", path)
		return false
	}
	fmt.Fprintf(stdout, "   ✅ %s（%d 个键，往返一致）\n", path, topKeyCount(plain, kind))
	return true
}

// topKeyCount 统计顶层键数（不计 sops 元数据键）
func topKeyCount(data []byte, kind string) int {
	document, err := models.ParseDocument(string(data), kind)
	if err != nil {
		return 0
	}
	return models.TopKeyCount(document)
}

func batchDecrypt(svc *service.Service, stdout, stderr io.Writer) int {
	if !svc.Capabilities().SecretsReady {
		fmt.Fprintln(stderr, "本机没有可用私钥，解不开 —— 在页面填写 age 私钥，或配置 SOPS_AGE_KEY / SOPS_AGE_KEY_FILE")
		return 1
	}
	reports, err := scan(svc)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	done, failure := 0, 0
	for _, report := range reports {
		if !report.readable {
			fmt.Fprintf(stderr, "   ❌ %s 读不到\n", report.path)
			failure++
			continue
		}
		if !report.encrypted {
			fmt.Fprintf(stdout, "   跳过 %s（已是明文）\n", report.path)
			continue
		}
		plain, err := decryptFile(svc, report.path)
		if err != nil {
			fmt.Fprintf(stderr, "   ❌ %s 解不开：%s\n", report.path, err)
			failure++
			continue
		}
		if err := repository.AtomicWrite(filepath.Join(svc.Root(), filepath.FromSlash(report.path)), plain); err != nil {
			fmt.Fprintf(stderr, "   ❌ %s 写入失败：%s\n", report.path, err)
			failure++
			continue
		}
		fmt.Fprintf(stdout, "   ✅ %s 已解开\n", report.path)
		done++
	}
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "解开 %d 份，失败 %d\n", done, failure)
	fmt.Fprintln(stdout, "⚠️  现在磁盘上是真凭据 改完务必跑 config-manager encrypt 加密回去，再提交")
	return boolToInt(failure != 0)
}

func batchShow(svc *service.Service, stdout, stderr io.Writer, args []string) int {
	if !svc.Capabilities().SecretsReady {
		fmt.Fprintln(stderr, "本机没有可用私钥，解不开 —— 在页面填写 age 私钥，或配置 SOPS_AGE_KEY / SOPS_AGE_KEY_FILE")
		return 1
	}
	if len(args) > 0 && args[0] != "" {
		plain, err := decryptFile(svc, args[0])
		if err != nil {
			fmt.Fprintf(stderr, "解不开 %s：%s\n", args[0], err)
			return 1
		}
		stdout.Write(plain)
		return 0
	}
	reports, err := scan(svc)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	for _, report := range reports {
		fmt.Fprintf(stdout, "── %s\n", report.path)
		data, err := os.ReadFile(filepath.Join(svc.Root(), filepath.FromSlash(report.path)))
		if err != nil {
			fmt.Fprintf(stderr, "   ❌ %s 读不到\n", report.path)
			continue
		}
		if !report.encrypted {
			for _, line := range strings.Split(string(data), "\n") {
				fmt.Fprintf(stdout, "   %s\n", line)
			}
			continue
		}
		plain, err := crypto.Decrypt(data, models.FileKind(report.path), svc.Identities())
		if err != nil {
			fmt.Fprintf(stderr, "   ❌ %s 解不开：%s\n", report.path, err)
			continue
		}
		for _, line := range strings.Split(strings.TrimRight(string(plain), "\n"), "\n") {
			fmt.Fprintf(stdout, "   %s\n", line)
		}
	}
	return 0
}

// decryptFile 读取并解密仓库里的一份密文文件
func decryptFile(svc *service.Service, file string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(svc.Root(), filepath.FromSlash(file)))
	if err != nil {
		return nil, err
	}
	return crypto.Decrypt(data, models.FileKind(file), svc.Identities())
}

// batchEdit 是命令行交互编辑：解到临时文件、打开编辑器、保存时重新加密
// 页面方式在内存中完成加解密，不产生编辑器临时明文文件
func batchEdit(svc *service.Service, file string) error {
	absolute := filepath.Join(svc.Root(), filepath.FromSlash(file))
	data, err := os.ReadFile(absolute)
	if err != nil {
		return fmt.Errorf("读不到 %s：%w", file, err)
	}
	kind := models.FileKind(file)
	plain := data
	if models.IsEncryptedContent(data, kind) {
		if !svc.Capabilities().SecretsReady {
			return fmt.Errorf("本机没有可用私钥，改不了 —— 在页面填写 age 私钥，或配置 SOPS_AGE_KEY / SOPS_AGE_KEY_FILE")
		}
		if plain, err = crypto.Decrypt(data, kind, svc.Identities()); err != nil {
			return fmt.Errorf("解不开 %s：%w", file, err)
		}
	}
	edited, err := editInEditor(plain, file)
	if err != nil {
		return err
	}
	if bytes.Equal(edited, plain) {
		fmt.Println("内容未修改，跳过")
		return nil
	}
	// 只重加密变化的值：编辑器里没动的项沿用旧密文，diff 干净可读
	output, err := crypto.EncryptSaving(svc.Root(), data, edited, kind, filepath.ToSlash(file), svc.Identities())
	if err != nil {
		return err
	}
	return repository.AtomicWrite(absolute, output)
}

// editInEditor 把明文写进临时文件并打开编辑器，返回编辑后的内容
func editInEditor(plain []byte, file string) ([]byte, error) {
	temporary, err := os.CreateTemp("", "config-edit-*"+filepath.Ext(file))
	if err != nil {
		return nil, err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err := temporary.Write(plain); err != nil {
		temporary.Close()
		return nil, err
	}
	if err := temporary.Close(); err != nil {
		return nil, err
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		if runtime.GOOS == "windows" {
			editor = "notepad"
		} else {
			editor = "vi"
		}
	}
	fmt.Printf("正在打开编辑器（%s）编辑 %s，保存退出后自动重新加密\n", editor, file)
	command := exec.Command(editor, name)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("编辑器退出异常：%w", err)
	}
	return os.ReadFile(name)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
