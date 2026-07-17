package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	chunkcache "github.com/ak/tts/chunk-cache"
	extractor "github.com/ak/tts/epub-text-extractor"
	converter "github.com/ak/tts/google-studio-tts-converter"
	merger "github.com/ak/tts/mp3-merger"
)

func readFileOrDie(path string) []byte {
	r, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read input file: %v\n", err)
		os.Exit(1)
	}
	return r
}

func sanitizeFilename(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "/", "_")
	return s
}

func main() {
	ctx := context.Background()
	flInput := flag.String("i", "", "input filename (required) epub or txt")
	flOutput := flag.String("o", ".", "output directory")
	flVoice := flag.String("voice", "Puck", "voice for TTS")
	flLanguage := flag.String("l", "en", "language for TTS")
	flModel := flag.String("model", "gemini-2.5-flash-preview-tts", "model for TTS")
	flPrompt := flag.String("prompt", "", "prompt prefix for TTS")
	flCache := flag.String("cache", "./cache/", "cache directory (optional) for TTS chunks")
	flChapter := flag.String("chapter", "", "chapter prefix and numeration for #-chapters")
	flMaxChunkSize := flag.Int("maxChunkSize", 2000, "max chunk size for epub extraction")
	flTitle := flag.String("intro", "intro", "mp3 file name of the first chapter receiving text before first * or # header")

	flag.Parse()

	if *flInput == "" {
		fmt.Fprintln(os.Stderr, "input file name required")
		flag.Usage()
		os.Exit(1)
	}

	var inputData []byte

	inputExt := strings.ToLower(filepath.Ext(*flInput))

	switch inputExt {
	case ".epub":
		txtPath := strings.TrimSuffix(*flInput, ".epub") + ".txt"
		if _, err := os.Stat(txtPath); err == nil {
			fmt.Printf("already converted, txt used instead of epub: %s\n", txtPath)
			inputData = readFileOrDie(txtPath)
		} else {
			fmt.Printf("converting epub to %s\n", txtPath)
			writer, err := os.Create(txtPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to create txt file: %v\n", err)
				os.Exit(1)
			}
			e := extractor.MakeEpubExtractor(writer, *flMaxChunkSize, *flTitle)
			if _, err := e.Write(readFileOrDie(*flInput)); err != nil {
				fmt.Fprintf(os.Stderr, "failed to extract epub: %v\n", err)
				os.Exit(1)
			}
			if err := e.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "failed to extract epub: %v\n", err)
				os.Exit(1)
			}
			inputData = readFileOrDie(txtPath)
		}
	case ".txt":
		inputData = readFileOrDie(*flInput)
	default:
		fmt.Fprintln(os.Stderr, "input should be epub or txt")
		os.Exit(1)
	}
	if err := os.MkdirAll(*flCache, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create cache dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(*flOutput, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create output dir: %v\n", err)
		os.Exit(1)
	}
	var conveyer io.WriteCloser
	scanner := bufio.NewScanner(bytes.NewReader(inputData))
	chapterCounter := 0
	for scanner.Scan() {
		line := scanner.Text()
		isVoiced := strings.HasPrefix(line, "#")
		if isVoiced || strings.HasPrefix(line, "*") {
			if conveyer != nil {
				if err := conveyer.Close(); err != nil {
					fmt.Fprintf(os.Stderr, "failed to encode chapter: %v\n", err)
					os.Exit(1)
				}
			}
			line := strings.TrimSpace(line[1:])
			outputName := sanitizeFilename(line)
			outputPath := filepath.Join(*flOutput, outputName+".mp3")
			fmt.Printf("starting chapter: %s\n", line)
			fileWriter, err := os.Create(outputPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to create output file: %v\n", err)
				os.Exit(1)
			}
			cache := chunkcache.MakeChunkCacheSink(
				*flCache, outputName, "mp3",
				merger.MakeMp3Joiner(fileWriter))
			conveyer = cache.MakeIntake(
				converter.MakeAiStudioTts(*flVoice, *flModel, *flLanguage, *flPrompt, cache, ctx))
			if !isVoiced {
				continue
			}
			if *flChapter != "" {
				chapterCounter++
				line = fmt.Sprintf("%s %d, %s", *flChapter, chapterCounter, line)
			}
		}
		if conveyer != nil {
			if _, err := conveyer.Write([]byte(line)); err != nil {
				fmt.Fprintf(os.Stderr, "failed to encode chunk: %v\n", err)
				os.Exit(1)
			}
		} else {
			fmt.Fprintln(os.Stderr, "error: text without *-line or #-line header")
			os.Exit(1)
		}
	}
	if conveyer != nil {
		if err := conveyer.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to finalize chapter: %v\n", err)
			os.Exit(1)
		}
	}
	fmt.Println("done")
}
