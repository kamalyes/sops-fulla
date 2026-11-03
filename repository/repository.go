/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 15:23:11
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 17:05:38
 * @FilePath: \sops-fulla\repository\repository.go
 * @Description: 配置仓库访问：目录扫描（兼容任意深度放置）与文件类别约定
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package repository

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kamalyes/sops-fulla/constants"
	"github.com/kamalyes/sops-fulla/models"
)

// Repository 负责仓库的目录扫描、路径解析与文件读写约定
type Repository struct {
	// Root 仓库根目录的绝对路径（已解析符号链接）
	Root string
	// secretsName 密钥文件的约定名
	secretsName string
}

// New 建立仓库句柄：解析绝对路径并消解符号链接
func New(root string) *Repository {
	absolute, err := filepath.Abs(root)
	if err == nil {
		root = absolute
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return &Repository{Root: root, secretsName: constants.DefaultSecretsName}
}

// SetSecretsName 换掉密钥文件名的约定（例如仓库结构改用
// credentials.enc.yaml）默认约定名之外的名字同样会被当作密钥文件
// 对待；只应在服务启动前调用
func (r *Repository) SetSecretsName(name string) {
	r.secretsName = name
}

// SecretsName 返回当前密钥文件约定名
func (r *Repository) SecretsName() string {
	return r.secretsName
}

// IsSecretsFile 判断是否密钥文件：默认约定名（含 yml/json 变体），
// 或通过 SetSecretsName 配置的约定名
func (r *Repository) IsSecretsFile(filename string) bool {
	switch filename {
	case "secrets.enc.yaml", "secrets.enc.yml", "secrets.enc.json":
		return true
	}
	return filename == r.secretsName && filename != "secrets.enc.yaml"
}

// FileKindName 返回目录展示用的文件类别：secrets（密钥文件）/ config（普通配置）
func (r *Repository) FileKindName(filename string) string {
	if r.IsSecretsFile(filename) {
		return "secrets"
	}
	return "config"
}

// catalogEntry 扫描过程中记录的文件条目
type catalogEntry struct {
	service     string
	environment string
	entry       models.FileEntry
}

// Catalog 递归扫描整个仓库：不限制目录层级，密钥文件可以放在任意位置
// 隐藏目录（.git、.github 等）与隐藏文件自动跳过
// 展示上按「服务 / 环境」分组：服务 = 第一级目录，环境 = 其余目录路径
// （可为多级，如 team/dev；直接放在服务目录或仓库根目录的文件环境名为空）
func (r *Repository) Catalog() models.Catalog {
	var found []catalogEntry
	r.walkConfigFiles(func(absPath, name string) {
		service, environment := groupOf(r.Root, absPath)
		found = append(found, catalogEntry{
			service:     service,
			environment: environment,
			entry:       models.FileEntry{Name: name, Kind: r.FileKindName(name), Exists: true},
		})
	})

	// 按（服务, 环境）分组
	type group struct {
		files []models.FileEntry
	}
	groups := map[string]map[string]*group{}
	for _, item := range found {
		if groups[item.service] == nil {
			groups[item.service] = map[string]*group{}
		}
		if groups[item.service][item.environment] == nil {
			groups[item.service][item.environment] = &group{files: []models.FileEntry{}}
		}
		groups[item.service][item.environment].files = append(groups[item.service][item.environment].files, item.entry)
	}

	// 排序并补充「待添加」的密钥文件条目
	catalog := models.Catalog{Services: []models.Service{}}
	serviceNames := make([]string, 0, len(groups))
	for name := range groups {
		serviceNames = append(serviceNames, name)
	}
	sort.Strings(serviceNames)
	for _, serviceName := range serviceNames {
		service := models.Service{Name: serviceName, Environments: []models.Environment{}}
		environmentNames := make([]string, 0, len(groups[serviceName]))
		for name := range groups[serviceName] {
			environmentNames = append(environmentNames, name)
		}
		sort.Strings(environmentNames)
		for _, environmentName := range environmentNames {
			files := groups[serviceName][environmentName].files
			sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
			if !hasSecretsEntry(files) {
				files = append(files, models.FileEntry{Name: r.secretsName, Kind: "secrets", Exists: false})
			}
			service.Environments = append(service.Environments, models.Environment{Name: environmentName, Files: files})
		}
		catalog.Services = append(catalog.Services, service)
	}
	return catalog
}

// walkConfigFiles 按仓库约定遍历所有可见的配置文件，供目录扫描与
// 密文清点共用：跳过隐藏目录 / 隐藏文件、非法名字与非 yaml/yml/json
func (r *Repository) walkConfigFiles(visit func(absPath, name string)) {
	_ = filepath.WalkDir(r.Root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			if path == r.Root {
				return nil
			}
			if strings.HasPrefix(name, ".") || !validName(name) {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || strings.HasPrefix(name, ".") || !validName(name) {
			return nil
		}
		switch strings.ToLower(filepath.Ext(name)) {
		case ".yaml", ".yml", ".json":
		default:
			return nil
		}
		visit(path, name)
		return nil
	})
}

// EncryptedFiles 递归列出仓库里所有处于密文状态的 *.enc.* 文件
// （绝对路径，按路径排序）：名字带 .enc. 但内容还是明文的会被排除
func (r *Repository) EncryptedFiles() []string {
	var found []string
	r.walkConfigFiles(func(absPath, name string) {
		if !models.IsEncryptedFile(name) {
			return
		}
		if raw, ok := r.ReadFile(absPath); ok && models.IsEncryptedContent(raw, models.FileKind(name)) {
			found = append(found, absPath)
		}
	})
	sort.Strings(found)
	return found
}

// groupOf 根据文件所在目录计算（服务, 环境）分组
// 统一用斜杠语义处理相对路径，避免 Windows 下 filepath.Dir
// 把分隔符规范化成反斜杠
func groupOf(root, pathArg string) (string, string) {
	relative, err := filepath.Rel(root, pathArg)
	if err != nil {
		return "", ""
	}
	dir := path.Dir(filepath.ToSlash(relative))
	if dir == "." {
		// 仓库根目录下的文件：服务与环境都为空
		return "", ""
	}
	segments := strings.Split(dir, "/")
	return segments[0], strings.Join(segments[1:], "/")
}

// hasSecretsEntry 判断文件列表里是否已有密钥文件
func hasSecretsEntry(files []models.FileEntry) bool {
	for _, file := range files {
		if file.Kind == "secrets" {
			return true
		}
	}
	return false
}

// ReadFile 读取普通文件内容；文件不存在或不是普通文件时返回 false
func (r *Repository) ReadFile(path string) ([]byte, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return data, true
}
