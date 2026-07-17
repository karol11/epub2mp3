package chunkcache

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
)

type CacheSink struct {
	dir     string
	key     string
	ext     string
	sink    io.WriteCloser
	nested  io.WriteCloser
	counter int
}

func MakeChunkCacheSink(dir, key string, ext string, sink io.WriteCloser) *CacheSink {
	return &CacheSink{
		dir:  dir,
		key:  key,
		ext:  ext,
		sink: sink,
	}
}

func (c *CacheSink) MakeIntake(wrapped io.WriteCloser) io.WriteCloser {
	c.nested = wrapped
	return &cacheIntake{c: c}
}

func (c *CacheSink) Write(p []byte) (n int, err error) {
	filename := c.getKeyFile()
	if err := os.WriteFile(filename, p, 0644); err != nil {
		log.Printf("can't write file %s\n", filename)
	}
	return c.sink.Write(p)
}

func (cw *CacheSink) Close() error { return nil } // Cache intake closes everything

func (c *CacheSink) getKeyFile() string {
	return filepath.Join(c.dir, c.key+"_"+strconv.Itoa(c.counter)+"."+c.ext)
}

type cacheIntake struct {
	c *CacheSink
}

func (c *cacheIntake) Close() error {
	if err := c.c.nested.Close(); err != nil {
		return err
	}
	return c.c.sink.Close()
}

func (c *cacheIntake) Write(p []byte) (n int, err error) {
	c.c.counter++
	filename := c.c.getKeyFile()
	if _, statErr := os.Stat(filename); statErr == nil {
		data, readErr := os.ReadFile(filename)
		if readErr == nil {
			return c.c.sink.Write(data)
		}
		log.Printf("can't read file %s\n", filename)
	}
	return c.c.nested.Write(p)
}
