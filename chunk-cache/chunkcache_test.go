package chunkcache

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type MockSink struct{ b bytes.Buffer }

func (m *MockSink) Write(p []byte) (n int, err error) {
	return m.b.Write(p)
}
func (m *MockSink) Close() error { return nil }

type MockNested struct{ s io.WriteCloser }

func (m *MockNested) Write(p []byte) (n int, err error) {
	out := make([]byte, len(p))
	for i, b := range p {
		out[i] = b + 1
	}
	return m.s.Write(out)
}
func (m *MockNested) Close() error { return nil }

func TestCachingWriterWritesToCacheAndSink(t *testing.T) {
	dir := t.TempDir()
	var sink MockSink
	cache := MakeChunkCacheSink(dir, "testkey", "txt", &sink)
	intake := cache.MakeIntake(&MockNested{s: cache})
	n, err := intake.Write([]byte("ABC"))
	if err != nil {
		t.Fatalf("postWriter.Write: %v", err)
	}
	if n != 3 {
		t.Fatalf("postWriter.Write returned %d, want %d", n, 3)
	}
	if sink.b.String() != "BCD" {
		t.Fatalf("sink got %q, want %q", sink.b.String(), "BCD")
	}
	cacheFile := filepath.Join(dir, "testkey_1.txt")
	disk, err := os.ReadFile(cacheFile)
	if err != nil {
		t.Fatalf("os.ReadFile: %v", err)
	}
	if string(disk) != "BCD" {
		t.Fatalf("cache file got %q, want %q", disk, "BCD")
	}
}

// TODO: add hit-test etc.
