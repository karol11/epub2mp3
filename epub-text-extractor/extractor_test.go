package extractor

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

type nopCloser struct {
	*bytes.Buffer
}

func (n *nopCloser) Close() error { return nil }

func createTestZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.Write([]byte(content))
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func parseZip(t *testing.T, data []byte) (*zip.Reader, error) {
	t.Helper()
	return zip.NewReader(bytes.NewReader(data), int64(len(data)))
}

func TestMakeEpubExtractor(t *testing.T) {
	buf := &nopCloser{Buffer: &bytes.Buffer{}}
	extractor := MakeEpubExtractor(buf, 1000, "")
	if extractor == nil {
		t.Fatal("expected non-nil extractor")
	}
}

func TestEpubExtractor_Write(t *testing.T) {
	buf := &nopCloser{Buffer: &bytes.Buffer{}}
	extractor := MakeEpubExtractor(buf, 1000, "").(*EpubExtractor)

	data := []byte("test epub content")
	n, err := extractor.Write(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("expected %d, got %d", len(data), n)
	}
	if len(extractor.data) != len(data) {
		t.Fatalf("expected data buffer to have %d bytes, got %d", len(data), len(extractor.data))
	}
}

func TestFindBookContent_FindsLargestHTML(t *testing.T) {
	html1 := `<html><body><p>small</p></body></html>`
	html2 := `<html><body><p>larger content with more text to make it bigger</p></body></html>`

	testZip := createTestZip(t, map[string]string{
		"content.xhtml":   html1,
		"chapter01.xhtml": html2,
		"nav.xhtml":       "<html><body>nav</body></html>",
		"cover.xhtml":     "<html><body>cover</body></html>",
	})

	r, err := parseZip(t, testZip)
	if err != nil {
		t.Fatal(err)
	}

	result := findBookContent(r)
	if result == nil {
		t.Fatal("expected to find content file")
	}
	if !strings.HasSuffix(result.Name, "chapter01.xhtml") {
		t.Fatalf("expected chapter01.xhtml, got %s", result.Name)
	}
}

func TestFindBookContent_SkipsNavAndCover(t *testing.T) {
	navOnly := `<html><body><p>nav content</p></body></html>`

	testZip := createTestZip(t, map[string]string{
		"nav.xhtml":   navOnly,
		"cover.xhtml": navOnly,
	})

	r, err := parseZip(t, testZip)
	if err != nil {
		t.Fatal(err)
	}

	result := findBookContent(r)
	if result != nil {
		t.Fatalf("expected nil (nav and cover only), got %s", result.Name)
	}
}

func TestExtractSnippets_H2WithChapter(t *testing.T) {
	extractor := MakeEpubExtractor(&nopCloser{}, 1000, "").(*EpubExtractor)

	html := `<html><body><h2>Introduction</h2></body></html>`
	snippets := extractor.extractSnippets(html)

	if len(snippets) != 1 {
		t.Fatalf("expected 1 snippet, got %d", len(snippets))
	}
	expected := "[narrator style][pause=1s]Chapter 1 Introduction[pause=1s][normal]\n"
	if snippets[0] != expected {
		t.Fatalf("expected %q, got %q", expected, snippets[0])
	}
}

func TestExtractSnippets_H2WithoutChapter(t *testing.T) {
	extractor := MakeEpubExtractor(&nopCloser{}, 1000, "").(*EpubExtractor)

	html := `<html><body><h2>Introduction</h2></body></html>`
	snippets := extractor.extractSnippets(html)

	if len(snippets) != 1 {
		t.Fatalf("expected 1 snippet, got %d", len(snippets))
	}
	expected := "Introduction.\n"
	if snippets[0] != expected {
		t.Fatalf("expected %q, got %q", expected, snippets[0])
	}
}

func TestExtractSnippets_H1H3H4(t *testing.T) {
	extractor := MakeEpubExtractor(&nopCloser{}, 1000, "").(*EpubExtractor)

	html := `<html><body>
		<h1>Heading 1</h1>
		<h3>Heading 3</h3>
		<h4>Heading 4</h4>
	</body></html>`

	snippets := extractor.extractSnippets(html)

	if len(snippets) != 3 {
		t.Fatalf("expected 3 snippets, got %d", len(snippets))
	}
	expectedH1 := "[narrator style][pause=300ms]Heading 1[pause=300ms][normal]\n"
	expectedH3 := "[narrator style][pause=300ms]Heading 3[pause=300ms][normal]\n"
	expectedH4 := "[narrator style][pause=300ms]Heading 4[pause=300ms][normal]\n"

	if snippets[0] != expectedH1 {
		t.Errorf("expected %q, got %q", expectedH1, snippets[0])
	}
	if snippets[1] != expectedH3 {
		t.Errorf("expected %q, got %q", expectedH3, snippets[1])
	}
	if snippets[2] != expectedH4 {
		t.Errorf("expected %q, got %q", expectedH4, snippets[2])
	}
}

func TestExtractSnippets_Paragraphs(t *testing.T) {
	extractor := MakeEpubExtractor(&nopCloser{}, 1000, "").(*EpubExtractor)

	html := `<html><body><p>First paragraph</p><p>Second paragraph</p></body></html>`
	snippets := extractor.extractSnippets(html)

	if len(snippets) != 2 {
		t.Fatalf("expected 2 snippets, got %d", len(snippets))
	}
	expected := "First paragraph\n"
	if snippets[0] != expected {
		t.Fatalf("expected %q, got %q", expected, snippets[0])
	}
}

func TestExtractSnippets_SkipsEmpty(t *testing.T) {
	extractor := MakeEpubExtractor(&nopCloser{}, 1000, "").(*EpubExtractor)

	html := `<html><body><h2></h2><p></p><h2>Valid</h2></body></html>`
	snippets := extractor.extractSnippets(html)

	if len(snippets) != 1 {
		t.Fatalf("expected 1 snippet (non-empty), got %d", len(snippets))
	}
}

func TestExtractSnippets_MultipleChapters(t *testing.T) {
	extractor := MakeEpubExtractor(&nopCloser{}, 1000, "").(*EpubExtractor)

	html := `<html><body>
		<h2>Chapter One</h2>
		<h2>Chapter Two</h2>
		<h2>Chapter Three</h2>
	</body></html>`

	snippets := extractor.extractSnippets(html)

	if len(snippets) != 3 {
		t.Fatalf("expected 3 snippets, got %d", len(snippets))
	}
	expected1 := "[narrator style][pause=1s]Part 1 Chapter One[pause=1s][normal]\n"
	expected2 := "[narrator style][pause=1s]Part 2 Chapter Two[pause=1s][normal]\n"
	expected3 := "[narrator style][pause=1s]Part 3 Chapter Three[pause=1s][normal]\n"

	if snippets[0] != expected1 {
		t.Errorf("expected %q, got %q", expected1, snippets[0])
	}
	if snippets[1] != expected2 {
		t.Errorf("expected %q, got %q", expected2, snippets[1])
	}
	if snippets[2] != expected3 {
		t.Errorf("expected %q, got %q", expected3, snippets[2])
	}
}

func TestPackChunks_SingleChunk(t *testing.T) {
	buf := &nopCloser{Buffer: &bytes.Buffer{}}
	extractor := MakeEpubExtractor(buf, 1000, "").(*EpubExtractor)

	snippets := []string{"hello world\n"}
	err := extractor.packChunks(snippets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.String() != "hello world\n" {
		t.Fatalf("expected 'hello world\\n', got %q", buf.String())
	}
}

func TestPackChunks_MultipleChunks(t *testing.T) {
	buf := &nopCloser{Buffer: &bytes.Buffer{}}
	extractor := MakeEpubExtractor(buf, 10, "").(*EpubExtractor)

	snippets := []string{"one\n", "two\n", "three\n"}
	err := extractor.packChunks(snippets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "one\n") {
		t.Errorf("expected output to contain 'one\\n', got %q", output)
	}
	if !strings.Contains(output, "two\n") {
		t.Errorf("expected output to contain 'two\\n', got %q", output)
	}
	if !strings.Contains(output, "three\n") {
		t.Errorf("expected output to contain 'three\\n', got %q", output)
	}
}

func TestPackChunks_EmptySnippets(t *testing.T) {
	buf := &nopCloser{Buffer: &bytes.Buffer{}}
	extractor := MakeEpubExtractor(buf, 1000, "").(*EpubExtractor)

	err := extractor.packChunks([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected empty buffer, got %q", buf.String())
	}
}

func TestClose_InvalidZip(t *testing.T) {
	buf := &nopCloser{Buffer: &bytes.Buffer{}}
	extractor := MakeEpubExtractor(buf, 1000, "").(*EpubExtractor)
	extractor.data = []byte("not a valid zip")

	err := extractor.Close()
	if err == nil {
		t.Fatal("expected error for invalid zip")
	}
}

func TestClose_NoBookContent(t *testing.T) {
	testZip := createTestZip(t, map[string]string{
		"nav.xhtml":   "<html><body>nav</body></html>",
		"cover.xhtml": "<html><body>cover</body></html>",
	})

	buf := &nopCloser{Buffer: &bytes.Buffer{}}
	extractor := MakeEpubExtractor(buf, 1000, "").(*EpubExtractor)
	extractor.data = testZip

	err := extractor.Close()
	if err == nil {
		t.Fatal("expected error for no book content")
	}
}

func TestClose_ValidEpub(t *testing.T) {
	html := `<html><body><h2>Test Chapter</h2><p>Test content</p></body></html>`
	testZip := createTestZip(t, map[string]string{
		"chapter.xhtml": html,
	})

	buf := &nopCloser{Buffer: &bytes.Buffer{}}
	extractor := MakeEpubExtractor(buf, 1000, "").(*EpubExtractor)
	extractor.data = testZip

	err := extractor.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "Test Chapter") {
		t.Errorf("expected output to contain 'Test Chapter', got %q", buf.String())
	}
}

func TestIntegration_SampleEpub(t *testing.T) {
	sampleData, err := os.ReadFile("../test-data/Gliding_on_Waves_EN.sample.epub")
	if err != nil {
		t.Skipf("skipping integration test: %v", err)
	}

	buf := &nopCloser{Buffer: &bytes.Buffer{}}
	extractor := MakeEpubExtractor(buf, 10000, "")

	n, err := extractor.Write(sampleData)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(sampleData) {
		t.Fatalf("expected %d bytes written, got %d", len(sampleData), n)
	}

	err = extractor.Close()
	if err != nil {
		t.Fatalf("Close error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Fatal("expected non-empty output")
	}

	if !strings.Contains(output, "[narrator style]") {
		t.Error("expected output to contain narrator style commands")
	}
}

type failWriter struct {
	err error
}

func (f *failWriter) Write(p []byte) (int, error) {
	return 0, f.err
}

func (f *failWriter) Close() error {
	return nil
}

func TestPackChunks_WriteError(t *testing.T) {
	failer := &failWriter{err: io.ErrClosedPipe}
	extractor := MakeEpubExtractor(failer, 10, "").(*EpubExtractor)

	snippets := []string{"one\n", "two\n"}
	err := extractor.packChunks(snippets)
	if err == nil {
		t.Fatal("expected error from write")
	}
}
