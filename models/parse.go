/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 13:58:20
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 15:09:33
 * @FilePath: \sops-fulla\models\parse.go
 * @Description: 通用文档解析：YAML/JSON 归一为保序 Value，含重复键与合并键处理
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package models

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/kamalyes/sops-fulla/errors"
	"gopkg.in/yaml.v3"
)

var (
	yamlLineRe = regexp.MustCompile(`line (\d+)`)
)

// FileKind 按扩展名识别文件格式：json 或 yaml
func FileKind(filename string) string {
	if filepath.Ext(filename) == ".json" {
		return "json"
	}
	return "yaml"
}

// IsEncryptedFile 通过文件名里的 ".enc." 判断是否为 SOPS 加密文件
func IsEncryptedFile(filename string) bool {
	return strings.Contains(filename, ".enc.")
}

// IsEncryptedContent 只看形态：顶层有没有 sops 元数据键不解密、不需要私钥
func IsEncryptedContent(data []byte, kind string) bool {
	document, err := ParseDocument(string(data), kind)
	if err != nil {
		return false
	}
	return HasTopKey(document, "sops")
}

func yamlLineHint(err error) string {
	if match := yamlLineRe.FindStringSubmatch(err.Error()); match != nil {
		return "，请检查第 " + match[1] + " 行"
	}
	return ""
}

func jsonLineHint(data []byte, err error) string {
	var syntax *json.SyntaxError
	if stderrors.As(err, &syntax) && syntax.Offset >= 0 && syntax.Offset <= int64(len(data)) {
		return fmt.Sprintf("，请检查第 %d 行", bytes.Count(data[:syntax.Offset], []byte("\n"))+1)
	}
	return ""
}

// ParseDocument 按格式解析文本并要求顶层是映射；空文档返回错误
func ParseDocument(text string, kind string) (*Value, error) {
	value, err := parseYAML(text)
	if kind == "json" {
		value, err = ParseJSON(text)
	}
	if err != nil {
		return nil, err
	}
	if value == nil || value.Kind != KindMapping {
		return nil, errors.New(400, errors.BizErrCodeInvalidDocument, "配置必须是键值映射，不能是空文档或列表")
	}
	return value, nil
}

// parseYAML 解析 YAML 文本：拒绝重复键，解析锚点别名并应用 << 合并键
func parseYAML(text string) (*Value, error) {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(text), &document); err != nil {
		return nil, errors.Newf(400, errors.BizErrCodeInvalidDocument, "配置格式有误%s", yamlLineHint(err))
	}
	root := &document
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			return nil, nil
		}
		root = root.Content[0]
	}
	if root.Kind == 0 {
		return nil, nil
	}
	if err := checkDuplicateKeys(root); err != nil {
		return nil, err
	}
	return nodeValue(root)
}

// isMergeKey 识别 YAML 的 << 合并键（不同版本标签写法不同，两种都认）
func isMergeKey(key *yaml.Node) bool {
	return key.Kind == yaml.ScalarNode && (key.Tag == "!!merge" || key.Value == "<<")
}

// checkDuplicateKeys 拒绝映射里的重复键（合并键除外）
func checkDuplicateKeys(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		seen := make(map[string]struct{}, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			if isMergeKey(key) {
				continue // 合并键允许与同名显式键共存
			}
			if key.Kind != yaml.ScalarNode {
				return errors.New(400, errors.BizErrCodeInvalidDocument, "配置键必须是普通名称")
			}
			if _, duplicate := seen[key.Value]; duplicate {
				return errors.New(400, errors.BizErrCodeInvalidDocument, "配置中存在重复的键，请合并后保存")
			}
			seen[key.Value] = struct{}{}
			if err := checkDuplicateKeys(node.Content[i+1]); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, item := range node.Content {
			if err := checkDuplicateKeys(item); err != nil {
				return err
			}
		}
	}
	return nil
}

func scalarKind(tag string) Kind {
	switch tag {
	case "!!bool":
		return KindBool
	case "!!int", "!!float":
		return KindNumber
	case "!!null":
		return KindNull
	default:
		return KindString
	}
}

// nodeValue 把 YAML 节点转成 Value：锚点解引用，合并键展开
func nodeValue(node *yaml.Node) (*Value, error) {
	switch node.Kind {
	case 0:
		return nil, nil
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return nil, nil
		}
		return nodeValue(node.Content[0])
	case yaml.AliasNode:
		// 解引用：往返比较只看内容，不看锚点写法
		return nodeValue(node.Alias)
	case yaml.ScalarNode:
		return &Value{Kind: scalarKind(node.Tag), Tag: node.Tag, Raw: node.Value}, nil
	case yaml.SequenceNode:
		value := &Value{Kind: KindSequence, Tag: "!!seq"}
		for _, item := range node.Content {
			child, err := nodeValue(item)
			if err != nil {
				return nil, err
			}
			value.Seq = append(value.Seq, child)
		}
		return value, nil
	case yaml.MappingNode:
		value := &Value{Kind: KindMapping, Tag: "!!map"}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, item := node.Content[i], node.Content[i+1]
			if isMergeKey(key) {
				merged, err := mergeEntries(item)
				if err != nil {
					return nil, err
				}
				for _, entry := range merged {
					if FindEntry(value, entry.Key) == nil {
						value.Map = append(value.Map, entry)
					}
				}
				continue
			}
			if key.Kind != yaml.ScalarNode {
				return nil, errors.New(400, errors.BizErrCodeInvalidDocument, "配置键必须是普通名称")
			}
			child, err := nodeValue(item)
			if err != nil {
				return nil, err
			}
			if existing := FindEntry(value, key.Value); existing != nil {
				existing.Value = child // 显式键覆盖同名的合并键
			} else {
				value.Map = append(value.Map, Entry{Key: key.Value, Value: child})
			}
		}
		return value, nil
	}
	return nil, errors.New(400, errors.BizErrCodeInvalidDocument, "配置格式有误")
}

// mergeEntries 展开合并键的取值：一个映射，或映射组成的序列
func mergeEntries(node *yaml.Node) ([]Entry, error) {
	if node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	switch node.Kind {
	case yaml.MappingNode:
		merged, err := nodeValue(node)
		if err != nil {
			return nil, err
		}
		return merged.Map, nil
	case yaml.SequenceNode:
		var entries []Entry
		for _, item := range node.Content {
			nested, err := mergeEntries(item)
			if err != nil {
				return nil, err
			}
			entries = append(entries, nested...)
		}
		return entries, nil
	}
	return nil, errors.New(400, errors.BizErrCodeInvalidDocument, "配置格式有误")
}

// ParseJSON 解析 JSON 文本，保持键顺序并拒绝重复键
func ParseJSON(text string) (*Value, error) {
	data := []byte(text)
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	value, err := decodeJSON(decoder)
	if err != nil {
		// 「重复键」原样透传，调用方靠错误码识别
		if errors.IsCode(err, errors.BizErrCodeDuplicateKey) {
			return nil, err
		}
		if stderrors.Is(err, io.EOF) {
			return nil, errors.New(400, errors.BizErrCodeInvalidDocument, "配置格式有误，请检查第 1 行")
		}
		return nil, errors.Newf(400, errors.BizErrCodeInvalidDocument, "配置格式有误%s", jsonLineHint(data, err))
	}
	if _, err := decoder.Token(); !stderrors.Is(err, io.EOF) {
		return nil, errors.New(400, errors.BizErrCodeInvalidDocument, "配置格式有误")
	}
	return value, nil
}

// decodeJSON 递归解码 JSON；重复键返回带错误码的业务错误
func decodeJSON(decoder *json.Decoder) (*Value, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	switch token := token.(type) {
	case json.Delim:
		switch token {
		case '{':
			value := &Value{Kind: KindMapping, Tag: "!!map"}
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, errors.New(400, errors.BizErrCodeInvalidDocument, "配置格式有误")
				}
				if _, duplicate := seen[key]; duplicate {
					return nil, errors.New(400, errors.BizErrCodeDuplicateKey, "JSON 中存在重复的键")
				}
				seen[key] = struct{}{}
				child, err := decodeJSON(decoder)
				if err != nil {
					return nil, err
				}
				value.Map = append(value.Map, Entry{Key: key, Value: child})
			}
			if _, err := decoder.Token(); err != nil { // 消费收尾的 '}'
				return nil, err
			}
			return value, nil
		case '[':
			value := &Value{Kind: KindSequence, Tag: "!!seq"}
			for decoder.More() {
				child, err := decodeJSON(decoder)
				if err != nil {
					return nil, err
				}
				value.Seq = append(value.Seq, child)
			}
			if _, err := decoder.Token(); err != nil { // 消费收尾的 ']'
				return nil, err
			}
			return value, nil
		}
		return nil, errors.New(400, errors.BizErrCodeInvalidDocument, "配置格式有误")
	case string:
		return &Value{Kind: KindString, Tag: "!!str", Raw: token}, nil
	case json.Number:
		tag := "!!int"
		if strings.ContainsAny(token.String(), ".eE") {
			tag = "!!float"
		}
		return &Value{Kind: KindNumber, Tag: tag, Raw: token.String()}, nil
	case bool:
		return &Value{Kind: KindBool, Tag: "!!bool", Raw: strconv.FormatBool(token)}, nil
	case nil:
		return &Value{Kind: KindNull, Tag: "!!null"}, nil
	}
	return nil, errors.New(400, errors.BizErrCodeInvalidDocument, "配置格式有误")
}
