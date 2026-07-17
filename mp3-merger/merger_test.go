package merger

import (
	"bytes"
	"io"
	"testing"
)

type nopCloser struct {
	*bytes.Buffer
}

func (n *nopCloser) Close() error { return nil }

func makeID3v2(audio []byte) []byte {
	header := []byte("ID3\x04\x00\x00\x00\x00\x00\x00")
	return append(header, audio...)
}

func makeID3v1(audio []byte) []byte {
	footer := make([]byte, 128)
	copy(footer, "TAG")
	return append(audio, footer...)
}

func makeFullFile(audio []byte) []byte {
	return makeID3v2(makeID3v1(audio))
}

func TestStripID3v2Header_NoHeader(t *testing.T) {
	data := []byte("AUDIO")
	got := stripID3v2Header(data)
	if !bytes.Equal(got, data) {
		t.Fatalf("expected unchanged, got %v", got)
	}
}

func TestStripID3v2Header_TooShort(t *testing.T) {
	data := []byte("ID3")
	got := stripID3v2Header(data)
	if !bytes.Equal(got, data) {
		t.Fatalf("expected unchanged, got %v", got)
	}
}

func TestStripID3v2Header_ValidHeader(t *testing.T) {
	audio := []byte("AUDIO")
	data := makeID3v2(audio)
	got := stripID3v2Header(data)
	if !bytes.Equal(got, audio) {
		t.Fatalf("expected %v, got %v", audio, got)
	}
}

func TestStripID3v2Header_TagSizeExceedsData(t *testing.T) {
	header := []byte("ID3\x04\x00")
	data := append(header, []byte{0x7f, 0xff, 0xff, 0xff}...)
	data = append(data, []byte("AUDIO")...)
	got := stripID3v2Header(data)
	if !bytes.Equal(got, data) {
		t.Fatalf("expected unchanged, got %v", got)
	}
}

func TestStripID3v1Footer_NoFooter(t *testing.T) {
	data := []byte("AUDIO")
	got := stripID3v1Footer(data)
	if !bytes.Equal(got, data) {
		t.Fatalf("expected unchanged, got %v", got)
	}
}

func TestStripID3v1Footer_TooShort(t *testing.T) {
	data := []byte("ATO")
	got := stripID3v1Footer(data)
	if !bytes.Equal(got, data) {
		t.Fatalf("expected unchanged, got %v", got)
	}
}

func TestStripID3v1Footer_ValidFooter(t *testing.T) {
	audio := []byte("AUDIO")
	data := makeID3v1(audio)
	got := stripID3v1Footer(data)
	if !bytes.Equal(got, audio) {
		t.Fatalf("expected %v, got %v", audio, got)
	}
}

func TestStripID3v1Footer_WrongTagPosition(t *testing.T) {
	data := make([]byte, len("ATO")+125)
	copy(data, "ATO")
	got := stripID3v1Footer(data)
	if !bytes.Equal(got, data) {
		t.Fatalf("expected unchanged, got %v", got)
	}
}

func TestMakeMp3Joiner(t *testing.T) {
	buf := &nopCloser{Buffer: &bytes.Buffer{}}
	joiner := MakeMp3Joiner(buf)
	if joiner == nil {
		t.Fatal("expected non-nil joiner")
	}
}

func TestMp3Joiner_WriteEmpty(t *testing.T) {
	buf := &nopCloser{Buffer: &bytes.Buffer{}}
	j := MakeMp3Joiner(buf).(*Mp3Joiner)
	n, err := j.Write([]byte{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected empty buffer, got %v", buf.Bytes())
	}
}

func TestMp3Joiner_FirstWriteStripsID3v2(t *testing.T) {
	buf := &nopCloser{Buffer: &bytes.Buffer{}}
	j := MakeMp3Joiner(buf).(*Mp3Joiner)

	part := makeID3v2([]byte("AUDIO"))
	n, err := j.Write(part)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Fatalf("expected 5, got %d", n)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected nothing written yet, got %v", buf.Bytes())
	}
	if len(j.prev) != 5 {
		t.Fatalf("expected prev length 5, got %d", len(j.prev))
	}
	if !bytes.Equal(j.prev, []byte("AUDIO")) {
		t.Fatalf("expected prev to be AUDIO, got %v", j.prev)
	}
}

func TestMp3Joiner_SecondWriteStripsID3v1FromPrev(t *testing.T) {
	buf := &nopCloser{Buffer: &bytes.Buffer{}}
	j := MakeMp3Joiner(buf).(*Mp3Joiner)

	first := makeID3v2([]byte("AUDIO1"))
	second := []byte("AUDIO2")

	j.Write(first)
	j.Write(second)

	if !bytes.Equal(buf.Bytes(), []byte("AUDIO1")) {
		t.Fatalf("expected AUDIO1, got %v", buf.Bytes())
	}
}

func TestMp3Joiner_MultipleWrites(t *testing.T) {
	buf := &nopCloser{Buffer: &bytes.Buffer{}}
	j := MakeMp3Joiner(buf).(*Mp3Joiner)

	parts := [][]byte{
		makeID3v2([]byte("A1")),
		makeID3v1([]byte("A2")),
		makeID3v1([]byte("A3")),
		[]byte("A4"),
	}

	for _, p := range parts {
		j.Write(p)
	}

	expected := []byte("A1A2A3")
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Fatalf("expected %v, got %v", expected, buf.Bytes())
	}
}

func TestMp3Joiner_CloseWritesLastChunk(t *testing.T) {
	buf := &nopCloser{Buffer: &bytes.Buffer{}}
	j := MakeMp3Joiner(buf).(*Mp3Joiner)

	j.Write([]byte("A1"))
	j.Close()

	if !bytes.Equal(buf.Bytes(), []byte("A1")) {
		t.Fatalf("expected A1, got %v", buf.Bytes())
	}
}

func TestMp3Joiner_CloseWithEmptyPrev(t *testing.T) {
	buf := &nopCloser{Buffer: &bytes.Buffer{}}
	j := MakeMp3Joiner(buf).(*Mp3Joiner)

	j.Close()

	if buf.Len() != 0 {
		t.Fatalf("expected empty buffer, got %v", buf.Bytes())
	}
}

func TestMp3Joiner_CloseKeepsLastFooter(t *testing.T) {
	buf := &nopCloser{Buffer: &bytes.Buffer{}}
	j := MakeMp3Joiner(buf).(*Mp3Joiner)

	part := makeID3v1([]byte("LAST"))
	j.Write(part)
	j.Close()

	expected := makeID3v1([]byte("LAST"))
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Fatalf("expected %v, got %v", expected, buf.Bytes())
	}
}

func TestMp3Joiner_WriteErrorPropagated(t *testing.T) {
	errFail := io.ErrClosedPipe
	failer := &failWriter{err: errFail}
	j := MakeMp3Joiner(failer).(*Mp3Joiner)

	j.Write([]byte("A1"))
	_, err := j.Write([]byte("A2"))
	if err != errFail {
		t.Fatalf("expected %v, got %v", errFail, err)
	}
}

func TestMp3Joiner_FilePathScenario(t *testing.T) {
	buf := &nopCloser{Buffer: &bytes.Buffer{}}
	j := MakeMp3Joiner(buf).(*Mp3Joiner)

	file1 := makeFullFile([]byte("intro"))
	file2 := []byte("body")
	file3 := []byte("outro")

	j.Write(file1)
	j.Write(file2)
	j.Write(file3)
	j.Close()

	expected := []byte("intro" + "body" + "outro")
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Fatalf("expected %v, got %v", expected, buf.Bytes())
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
