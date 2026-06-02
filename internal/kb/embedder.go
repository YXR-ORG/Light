package kb

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sync"

	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/pipelines"
)

// modelDir 返回 embedding 模型目录。
// 优先用 app bundle 内置路径，其次用 chroma 缓存（开发环境）。
func modelDir() string {
	// 1. app bundle: Contents/Resources/models/all-MiniLM-L6-v2
	exe, _ := os.Executable()
	bundlePath := filepath.Join(filepath.Dir(exe), "..", "Resources", "models", "all-MiniLM-L6-v2")
	if _, err := os.Stat(filepath.Join(bundlePath, "tokenizer.json")); err == nil {
		return bundlePath
	}
	// 2. 开发环境：项目根目录 build/models/all-MiniLM-L6-v2
	devPath := filepath.Join(filepath.Dir(exe), "..", "..", "..", "..", "build", "models", "all-MiniLM-L6-v2")
	if _, err := os.Stat(filepath.Join(devPath, "tokenizer.json")); err == nil {
		return devPath
	}
	// 3. chroma 缓存（开发机）
	home, _ := os.UserHomeDir()
	chromaPath := filepath.Join(home, ".cache", "chroma", "onnx_models", "all-MiniLM-L6-v2", "onnx")
	if _, err := os.Stat(filepath.Join(chromaPath, "tokenizer.json")); err == nil {
		return chromaPath
	}
	return ""
}

// embedderSingleton 全局单例，避免重复加载模型
var (
	embedderOnce     sync.Once
	embedderPipeline *pipelines.FeatureExtractionPipeline
	embedderSession  *hugot.Session
	embedderErr      error
)

// getEmbedder 获取（懒加载）全局 embedding pipeline
func getEmbedder() (*pipelines.FeatureExtractionPipeline, error) {
	embedderOnce.Do(func() {
		dir := modelDir()
		if dir == "" {
			embedderErr = fmt.Errorf("embedding model not found; place all-MiniLM-L6-v2 in build/models/")
			return
		}
		slog.Info("embedder: loading model", "dir", dir)
		ctx := context.Background()
		sess, err := hugot.NewGoSession(ctx)
		if err != nil {
			embedderErr = fmt.Errorf("hugot NewGoSession: %w", err)
			return
		}
		p, err := hugot.NewPipeline(sess, hugot.FeatureExtractionConfig{
			ModelPath: dir,
		})
		if err != nil {
			sess.Destroy()
			embedderErr = fmt.Errorf("hugot NewPipeline: %w", err)
			return
		}
		embedderSession = sess
		embedderPipeline = p
		slog.Info("embedder: model loaded", "dir", dir)
	})
	return embedderPipeline, embedderErr
}

// Embed 把一批文本转成 384 维 float32 向量
func Embed(texts []string) ([][]float32, error) {
	p, err := getEmbedder()
	if err != nil {
		return nil, err
	}
	out, err := p.Run(context.Background(), texts)
	if err != nil {
		return nil, fmt.Errorf("embed run: %w", err)
	}
	raw := out.GetOutput()
	result := make([][]float32, len(raw))
	for i, v := range raw {
		vec, ok := v.([]float32)
		if !ok {
			return nil, fmt.Errorf("embed: unexpected output type %T", v)
		}
		result[i] = normalize(vec)
	}
	return result, nil
}

// normalize L2 归一化，使余弦相似度等价于点积
func normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	norm := float32(math.Sqrt(sum))
	if norm == 0 {
		return v
	}
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x / norm
	}
	return out
}

// Float32SliceToBytes 序列化 float32 切片为 little-endian bytes（存入 SQLite BLOB）
func Float32SliceToBytes(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

// BytesToFloat32Slice 反序列化
func BytesToFloat32Slice(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

// CosineSim 计算两个已归一化向量的点积（等价余弦相似度）
func CosineSim(a, b []float32) float32 {
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}
