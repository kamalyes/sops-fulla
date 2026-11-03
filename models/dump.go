/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 13:55:11
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 15:02:28
 * @FilePath: \sops-fulla\models\dump.go
 * @Description: 密钥键值对序列化回 YAML/JSON 文件文本
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package models

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

// DumpSecrets 把密钥对序列化回文件文本
func DumpSecrets(pairs []Entry, kind string) ([]byte, error) {
	if kind == "json" {
		return DumpSecretsJSON(pairs), nil
	}
	return dumpSecretsYAML(pairs)
}

// dumpSecretsYAML 值统一按字符串标量输出，保留键顺序
func dumpSecretsYAML(pairs []Entry) ([]byte, error) {
	mapping := &yaml.Node{Kind: yaml.MappingNode}
	for _, pair := range pairs {
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: pair.Key},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: pair.Value.Raw},
		)
	}
	document := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{mapping}}
	return yaml.Marshal(document)
}

// DumpSecretsJSON 输出扁平 JSON 对象，保留键顺序
func DumpSecretsJSON(pairs []Entry) []byte {
	var buffer bytes.Buffer
	buffer.WriteString("{")
	for i, pair := range pairs {
		if i > 0 {
			buffer.WriteString(",")
		}
		buffer.WriteString("\n  ")
		AppendJSONString(&buffer, pair.Key)
		buffer.WriteString(": ")
		AppendJSONString(&buffer, pair.Value.Raw)
	}
	if len(pairs) > 0 {
		buffer.WriteString("\n")
	}
	buffer.WriteString("}")
	return append(buffer.Bytes(), '\n')
}
