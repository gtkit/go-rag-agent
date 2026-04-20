package ragagent

// Document 表示加载后的源文档。
type Document struct {
	ID         string
	SourcePath string
	Title      string
	Metadata   map[string]string
	Content    string
}

// Chunk 表示切块后的文档片段。
type Chunk struct {
	ChunkID    string
	ParentID   string
	SourcePath string
	Title      string
	Metadata   map[string]string
	Text       string
	StartRune  int
	EndRune    int
}

// ChunkRecord 表示已索引的分块记录。
type ChunkRecord struct {
	ChunkID    string
	ParentID   string
	SourcePath string
	Title      string
	Text       string
	StartRune  int
	EndRune    int
	Metadata   map[string]string
	Embedding  []float32
}

// SearchHit 表示一次向量检索结果。
type SearchHit struct {
	Chunk ChunkRecord
	Score float32
}

// SearchFilter 定义向量检索时可选的来源与元数据过滤条件。
type SearchFilter struct {
	SourcePaths    []string
	SourcePrefixes []string
	Metadata       map[string]string
}
