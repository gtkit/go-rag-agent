package ragagent

import (
	"context"

	"github.com/gtkit/go-rag-agent/internal/rag"
)

// DocumentLoadOptions 定义文档加载时的可选能力。
type DocumentLoadOptions struct {
	PDFOCR             func(context.Context, string) (string, error)
	MinDirectTextRunes int
}

// DocumentLoader 将输入文件加载为标准文档。
type DocumentLoader interface {
	Load(ctx context.Context, path string, title string, metadata map[string]string, opts DocumentLoadOptions) (Document, error)
}

type fileDocumentLoader struct{}

// NewFileDocumentLoader 创建默认本地文件加载器。
func NewFileDocumentLoader() DocumentLoader {
	return fileDocumentLoader{}
}

func (fileDocumentLoader) Load(ctx context.Context, path string, title string, metadata map[string]string, opts DocumentLoadOptions) (Document, error) {
	doc, err := rag.LoadFile(ctx, path, title, metadata, rag.LoadOptions{
		PDFOCR:             opts.PDFOCR,
		MinDirectTextRunes: opts.MinDirectTextRunes,
	})
	if err != nil {
		return Document{}, err
	}
	return fromInternalDocument(doc), nil
}
