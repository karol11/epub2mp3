package merger

// Joins mp3 in one

import (
	"bytes"
	"io"
)

type Mp3Joiner struct {
	w    io.WriteCloser
	prev []byte
}

func MakeMp3Joiner(w io.WriteCloser) io.WriteCloser {
	return &Mp3Joiner{w: w}
}

// 3. Strip ID3v2 (header) and ID3v1 (footer) tags from all files
// except keeping the first file's header and last file's footer
func (j *Mp3Joiner) Write(part []byte) (int, error) {
	if len(part) == 0 {
		return 0, nil
	}
	if len(j.prev) == 0 { // it's first
		part = stripID3v2Header(part)
	}
	if n, err := j.w.Write(stripID3v1Footer(j.prev)); err != nil {
		return n, err
	}
	j.prev = part
	return len(part), nil
}
func (j *Mp3Joiner) Close() error {
	if len(j.prev) != 0 {
		if _, err := j.w.Write(j.prev); err != nil {
			return err
		}
	}
	j.w.Close()
	return nil
}

// stripID3v2 removes the ID3v2 header tag from the start of an MP3
func stripID3v2Header(data []byte) []byte {
	// ID3v2 header: "ID3" + 2 bytes version + 1 byte flags + 4 bytes size
	if len(data) < 10 || !bytes.HasPrefix(data, []byte("ID3")) {
		return data
	}
	// Size is encoded as 4 synchsafe integers
	size := int(data[6]&0x7f)<<21 |
		int(data[7]&0x7f)<<14 |
		int(data[8]&0x7f)<<7 |
		int(data[9]&0x7f)
	tagSize := 10 + size
	if tagSize > len(data) {
		return data
	}
	return data[tagSize:]
}

// stripID3v1 removes the ID3v1 footer tag from the end of an MP3
func stripID3v1Footer(data []byte) []byte {
	// ID3v1 tag is always exactly 128 bytes at the end, starting with "TAG"
	if len(data) >= 128 && bytes.Equal(data[len(data)-128:len(data)-125], []byte("TAG")) {
		return data[:len(data)-128]
	}
	return data
}
