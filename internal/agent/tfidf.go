package agent

import (
	"math"
	"sort"
	"strings"
)

// TFIDFIndex 是一个基于 TF-IDF 的全文检索索引
// 替代纯关键词匹配，提升 RAG 检索质量
type TFIDFIndex struct {
	documents []KnowledgeDoc
	// 每篇文档的词频统计
	docTermFreqs []map[string]int
	// 逆文档频率
	idf map[string]float64
	// 文档向量 (归一化后的 TF-IDF)
	docVectors []map[string]float64
	// 所有词汇表
	vocabulary map[string]bool
}

// NewTFIDFIndex 构建 TF-IDF 索引
func NewTFIDFIndex(docs []KnowledgeDoc) *TFIDFIndex {
	idx := &TFIDFIndex{
		documents:  docs,
		vocabulary: make(map[string]bool),
		idf:        make(map[string]float64),
	}

	// 1. 分词并统计词频
	idx.docTermFreqs = make([]map[string]int, len(docs))
	for i, doc := range docs {
		words := idx.tokenize(doc.Title + " " + doc.Content)
		tf := make(map[string]int)
		for _, w := range words {
			tf[w]++
		}
		idx.docTermFreqs[i] = tf
		for w := range tf {
			idx.vocabulary[w] = true
		}
	}

	// 2. 计算 IDF
	nDocs := float64(len(docs))
	for term := range idx.vocabulary {
		df := 0 // 包含该词的文档数
		for _, tf := range idx.docTermFreqs {
			if tf[term] > 0 {
				df++
			}
		}
		if df == 0 {
			idx.idf[term] = 0
		} else {
			idx.idf[term] = math.Log(nDocs / float64(df))
		}
	}

	// 3. 计算每篇文档的 TF-IDF 向量（归一化）
	idx.docVectors = make([]map[string]float64, len(docs))
	for i, tf := range idx.docTermFreqs {
		vec := make(map[string]float64)
		var sumSq float64
		for term, count := range tf {
			tfVal := float64(count)
			tfidf := tfVal * idx.idf[term]
			if tfidf > 0 {
				vec[term] = tfidf
				sumSq += tfidf * tfidf
			}
		}
		// L2 归一化
		if sumSq > 0 {
			norm := math.Sqrt(sumSq)
			for term, val := range vec {
				vec[term] = val / norm
			}
		}
		idx.docVectors[i] = vec
	}

	return idx
}

// tokenize 分词：英文按单词，中文按 2-gram
func (idx *TFIDFIndex) tokenize(text string) []string {
	return splitWords(strings.ToLower(text))
}

// Search 检索最相关的文档
func (idx *TFIDFIndex) Search(query string, topK int) []SearchResult {
	if topK <= 0 {
		topK = 5
	}

	// 构建查询向量
	queryWords := idx.tokenize(query)
	queryTF := make(map[string]int)
	for _, w := range queryWords {
		queryTF[w]++
	}

	queryVec := make(map[string]float64)
	var sumSq float64
	for term, count := range queryTF {
		tfVal := float64(count)
		idf := idx.idf[term]
		if idf == 0 {
			// 未见过的词，给一个默认 IDF
			idf = math.Log(float64(len(idx.documents)))
		}
		tfidf := tfVal * idf
		if tfidf > 0 {
			queryVec[term] = tfidf
			sumSq += tfidf * tfidf
		}
	}
	// L2 归一化查询向量
	if sumSq > 0 {
		norm := math.Sqrt(sumSq)
		for term, val := range queryVec {
			queryVec[term] = val / norm
		}
	}

	// 计算余弦相似度
	type scored struct {
		idx   int
		score float64
	}
	var results []scored
	for i, docVec := range idx.docVectors {
		var dotProduct float64
		// 遍历查询向量中的词（通常比文档向量短）
		for term, qVal := range queryVec {
			if dVal, ok := docVec[term]; ok {
				dotProduct += qVal * dVal
			}
		}
		if dotProduct > 0 {
			results = append(results, scored{idx: i, score: dotProduct})
		}
	}

	// 按分数排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	// 取 topK
	if len(results) > topK {
		results = results[:topK]
	}

	// 构建返回结果
	var searchResults []SearchResult
	for _, r := range results {
		doc := idx.documents[r.idx]
		// score 已是 0-1 范围的余弦相似度
		searchResults = append(searchResults, SearchResult{
			Title:   doc.Title,
			Content: extractSnippet(doc.Content, queryWords, 200),
			Source:  doc.Title,
			Score:   r.score,
		})
	}

	// 如果 TF-IDF 没有匹配，降级到简单匹配
	if len(searchResults) == 0 {
		queryLower := strings.ToLower(query)
		queryWordsSimple := splitWords(queryLower)
		for _, doc := range idx.documents {
			contentLower := strings.ToLower(doc.Content)
			titleLower := strings.ToLower(doc.Title)

			score := 0
			for _, w := range queryWordsSimple {
				if w == "" {
					continue
				}
				if strings.Contains(titleLower, w) {
					score += 5
				}
				count := strings.Count(contentLower, w)
				score += count
			}
			if score > 0 {
				searchResults = append(searchResults, SearchResult{
					Title:   doc.Title,
					Content: extractSnippet(doc.Content, queryWordsSimple, 200),
					Source:  doc.Title,
					Score:   float64(score) / 100, // 归一化到 0-1
				})
			}
		}
		sort.Slice(searchResults, func(i, j int) bool {
			return searchResults[i].Score > searchResults[j].Score
		})
		if len(searchResults) > topK {
			searchResults = searchResults[:topK]
		}
	}

	return searchResults
}
