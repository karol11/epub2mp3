package extractor

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
)

type EpubExtractor struct {
	w                io.WriteCloser
	maxChunkSize     int
	firstSectionName string
	data             []byte
}

func MakeEpubExtractor(w io.WriteCloser, maxChunkSize int, firstSectionName string) io.WriteCloser {
	return &EpubExtractor{w: w, maxChunkSize: maxChunkSize, data: []byte{}, firstSectionName: firstSectionName}
}

// Write implements [io.WriteCloser].
func (e *EpubExtractor) Write(p []byte) (n int, err error) {
	e.data = append(e.data, p...)
	return len(p), nil
}

// Close implements [io.WriteCloser].
func (e *EpubExtractor) Close() error {
	r, err := zip.NewReader(bytes.NewReader(e.data), int64(len(e.data)))
	if err != nil {
		return fmt.Errorf("failed to open epub: %w", err)
	}
	bookFile := findBookContent(r)
	if bookFile == nil {
		return fmt.Errorf("could not find main book content in epub")
	}
	rc, err := bookFile.Open()
	if err != nil {
		return fmt.Errorf("failed to open content file: %w", err)
	}
	defer rc.Close()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("failed to read content: %w", err)
	}
	snippets := e.extractSnippets(string(raw))
	err = e.packChunks(snippets)
	if err != nil {
		return err
	}
	return e.w.Close()
}

// findBookContent picks the largest xhtml file that isn't nav or cover.
func findBookContent(r *zip.Reader) *zip.File {
	var best *zip.File
	for _, f := range r.File {
		name := strings.ToLower(f.Name)
		if !strings.HasSuffix(name, ".xhtml") && !strings.HasSuffix(name, ".html") {
			continue
		}
		base := filepath.Base(name)
		if strings.Contains(base, "nav") || strings.Contains(base, "cover") {
			continue
		}
		if best == nil || f.UncompressedSize64 > best.UncompressedSize64 {
			best = f
		}
	}
	return best
}

// extractSnippets parses HTML and converts headings/paragraphs to styled snippets.
func (e *EpubExtractor) extractSnippets(content string) []string {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return nil
	}
	var snippets []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "h1", "h2":
				if text := strings.TrimSpace(nodeText(n)); text != "" {
					snippets = append(snippets, "# "+text+".\n")
				}
				return
			case "h3", "h4":
				if text := strings.TrimSpace(nodeText(n)); text != "" {
					snippets = append(snippets, "##"+text+".\n")
				}
				return
			case "p":
				if text := strings.TrimSpace(nodeText(n)); text != "" {
					if len(snippets) == 0 && e.firstSectionName != "" {
						if strings.HasPrefix(e.firstSectionName, "#") || strings.HasPrefix(e.firstSectionName, "*") {
							snippets = append(snippets, e.firstSectionName+"\n")
						} else {
							snippets = append(snippets, "*"+e.firstSectionName+"\n")
						}
					}
					snippets = append(snippets, text+"\n")
				}
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return snippets
}

// nodeText recursively extracts plain text from an HTML node.
func nodeText(n *html.Node) string {
	if n.Type == html.TextNode {
		return strings.Join(strings.Fields(n.Data), " ")
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(nodeText(c))
		//sb.WriteString(" ")
	}
	return sb.String()
}

// packChunks greedily packs snippets into style text documents each ≤ maxSize bytes truncating at #-headers.
func (e *EpubExtractor) packChunks(snippets []string) error {
	var currentBody strings.Builder
	flush := func() error {
		if currentBody.Len() == 0 {
			return nil
		}
		_, err := e.w.Write([]byte(currentBody.String()))
		currentBody.Reset()
		return err
	}

	for _, snippet := range snippets {
		snippetBytes := len(snippet)
		currentBytes := currentBody.Len()
		if strings.HasPrefix(snippet, "#") || currentBytes+snippetBytes > e.maxChunkSize && currentBody.Len() > 0 {
			if err := flush(); err != nil {
				return err
			}
		}
		currentBody.WriteString(snippet)
	}
	return flush()
}
