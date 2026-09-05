// Package rag 实现 RAG 知识库能力。
// 承载职位表、进面分数线、报录比、专业目录等数据的检索服务。
package rag

// Store 知识库存储与检索接口。
type Store interface {
	// Search 按查询条件检索知识库，返回命中的文档片段列表。
	Search(query string, topK int) ([]Document, error)
}

// Document 知识库文档片段。
type Document struct {
	ID      string  // 文档唯一标识
	Content string  // 文档内容
	Score   float64 // 相关度得分
}
