/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 18:37:05
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 19:53:28
 * @FilePath: \sops-fulla\service\service.go
 * @Description: 配置读写编排：路径解析、文档校验与最小diff加解密
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

// Package service 编排一次配置读写：把仓库、文档模型与 SOPS 加解密
// 组装成页面和命令行共用的读写视图
package service

import (
	"path/filepath"
	"strings"
	"sync"

	agekey "github.com/getsops/sops/v3/age"

	"github.com/kamalyes/sops-fulla/constants"
	"github.com/kamalyes/sops-fulla/crypto"
	"github.com/kamalyes/sops-fulla/errors"
	"github.com/kamalyes/sops-fulla/models"
	"github.com/kamalyes/sops-fulla/repository"
)

// Service 是配置仓库的读写视图
type Service struct {
	// repo 负责目录扫描、路径校验与文件读写约定
	repo *repository.Repository

	keyMu      sync.Mutex
	envSecret  string // 启动时从环境变量 / 密钥文件读到的一份
	envSource  string // env | file | ""
	pageSecret string // 页面提供的私钥，仅内存，不落盘
	parsedPage agekey.ParsedIdentities
	parsedEnv  agekey.ParsedIdentities // nil 表示尚未解析

	lock sync.Mutex // 保存串行化
}

// New 创建服务视图，并按 sops 的约定读取环境里的 age 私钥：
// SOPS_AGE_KEY、SOPS_AGE_KEY_FILE，以及标准的本机密钥文件路径
func New(root string) *Service {
	return newService(root, true)
}

// NewIsolated 创建不读取环境私钥的实例，让测试不受宿主机配置影响
func NewIsolated(root string) *Service {
	return newService(root, false)
}

func newService(root string, readEnvironment bool) *Service {
	svc := &Service{repo: repository.New(root)}
	if readEnvironment {
		svc.envSecret, svc.envSource = environmentKey()
	}
	return svc
}

// Root 返回仓库根目录的绝对路径
func (s *Service) Root() string { return s.repo.Root }

// SetSecretsName 换掉密钥文件名的约定（例如仓库结构改用
// credentials.enc.yaml）默认约定名之外的名字同样会被当作密钥文件
// 对待；只应在服务启动前调用
func (s *Service) SetSecretsName(name string) {
	s.repo.SetSecretsName(name)
}

// Catalog 扫描仓库目录树，附带当前的能力信息
func (s *Service) Catalog() models.Catalog {
	catalog := s.repo.Catalog()
	catalog.Capabilities = s.Capabilities()
	return catalog
}

// Read 读取一个配置文件；密文先在内存里解密，磁盘上的内容不动
// service / environment 可为空、environment 允许多级路径，兼容任意放置
func (s *Service) Read(service, environment, filename string) (*models.FileData, error) {
	target, err := s.repo.Resolve(service, environment, filename)
	if err != nil {
		return nil, err
	}
	kind := models.FileKind(filename)
	raw, exists := s.repo.ReadFile(target)
	data := raw
	if exists && models.IsEncryptedFile(filename) && models.IsEncryptedContent(raw, kind) {
		if data, err = crypto.Decrypt(raw, kind, s.Identities()); err != nil {
			return nil, err
		}
	}
	result := &models.FileData{Filename: filename, Kind: s.repo.FileKindName(filename)}
	if exists {
		revision := repository.RevisionOf(raw)
		result.Revision = &revision
	}
	text := string(data)
	if s.repo.IsSecretsFile(filename) {
		result.Values = &models.Value{Kind: models.KindMapping, Tag: "!!map", Map: []models.Entry{}}
		if text != "" {
			document, err := models.ParseDocument(text, kind)
			if err != nil {
				return nil, err
			}
			pairs, err := SecretPairs(document)
			if err != nil {
				return nil, err
			}
			result.Values.Map = pairs
		}
	} else {
		result.Content = &text
	}
	return result, nil
}

// Save 校验并写入一个配置文件；密文文件会先加密、解回来核对，一致才落盘
func (s *Service) Save(request *models.SaveRequest) (*models.SaveResult, error) {
	target, err := s.repo.Resolve(request.Service, request.Environment, request.Filename)
	if err != nil {
		return nil, err
	}
	kind := models.FileKind(request.Filename)

	s.lock.Lock()
	defer s.lock.Unlock()

	original, exists := s.repo.ReadFile(target)
	if !repository.SameRevision(original, exists, request.Revision) {
		return nil, conflict()
	}
	document, plain, err := s.prepare(request, kind)
	if err != nil {
		return nil, err
	}
	if len(plain) > constants.MaxBodyBytes {
		return nil, errors.New(413, errors.BizErrCodePayloadTooLarge, "配置文件过大")
	}

	output := plain
	if models.IsEncryptedFile(request.Filename) {
		relPath, err := relativePath(s.Root(), target)
		if err != nil {
			return nil, err
		}
		// 只重加密变化的值：未变的值沿用旧密文，diff 里只有真正改动的行
		output, err = crypto.EncryptSaving(s.Root(), original, plain, kind, relPath, s.Identities())
		if err != nil {
			return nil, err
		}
		// 解回来核对：密文落盘前确认内容没有变形（这一步需要私钥）
		verified, err := crypto.Decrypt(output, kind, s.Identities())
		if err != nil {
			return nil, err
		}
		verifiedDocument, err := models.ParseDocument(string(verified), kind)
		if err != nil || !document.Equal(verifiedDocument) {
			return nil, errors.New(500, errors.BizErrCodeVerifyFailed, "保存后内容校验不一致，原文件未修改")
		}
	}
	// 加密期间外部编辑器也可能修改文件，写入前再核对一次
	current, stillExists := s.repo.ReadFile(target)
	if !repository.SameRevision(current, stillExists, request.Revision) {
		return nil, conflict()
	}
	if err := repository.AtomicWrite(target, output); err != nil {
		return nil, err
	}
	return &models.SaveResult{
		Revision: repository.RevisionOf(output),
		Message:  "已保存，重新部署对应服务后生效",
	}, nil
}

// prepare 按文件类型把请求归一成（用于校验的文档, 待落盘明文）
func (s *Service) prepare(request *models.SaveRequest, kind string) (*models.Value, []byte, error) {
	if s.repo.IsSecretsFile(request.Filename) {
		if request.Values == nil {
			return nil, nil, errors.New(400, errors.BizErrCodeSecretsShape, "密钥必须是名称与值的映射")
		}
		pairs, err := SecretPairs(request.Values)
		if err != nil {
			return nil, nil, err
		}
		plain, err := models.DumpSecrets(pairs, kind)
		if err != nil {
			return nil, nil, err
		}
		return &models.Value{Kind: models.KindMapping, Tag: "!!map", Map: pairs}, plain, nil
	}
	if request.Content == nil {
		return nil, nil, errors.New(400, errors.BizErrCodeInvalidRequest, "配置内容必须是文本")
	}
	text := *request.Content
	document, err := models.ParseDocument(text, kind)
	if err != nil {
		return nil, nil, err
	}
	if strings.Contains(text, "PRIVATE KEY-----") {
		return nil, nil, errors.New(400, errors.BizErrCodePrivateKeyLocked, "私钥不能保存在普通配置里，请使用密钥配置")
	}
	return document, []byte(text), nil
}

// conflict 构造保存冲突（409）错误
func conflict() *errors.Error {
	return errors.New(409, errors.BizErrCodeRevisionConflict, "文件已被其他人修改，请重新加载后再保存")
}

// relativePath 算出仓库内的斜杠分隔相对路径
func relativePath(root, target string) (string, error) {
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return "", errors.New(500, errors.BizErrCodeInvalidRequest, "操作未完成，请刷新后重试")
	}
	return filepath.ToSlash(relative), nil
}
