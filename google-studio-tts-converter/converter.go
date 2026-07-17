package converter

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"log"

	"google.golang.org/genai"
)

type AiStudioTts struct {
	w      io.WriteCloser
	client *genai.Client
	ctx    context.Context
	model  string
	prompt string
	config genai.GenerateContentConfig
}

func MakeAiStudioTts(voice string, model string, language string, prompt string, w io.WriteCloser, ctx context.Context) io.WriteCloser {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		log.Fatalf("Failed to create GenAI client: %v", err)
	}
	return &AiStudioTts{
		w:      w,
		client: client,
		ctx:    ctx,
		model:  model,  //"gemini-3.1-flash-tts-preview"
		prompt: prompt, //"[cheerfully]"
		config: genai.GenerateContentConfig{
			ResponseModalities: []string{"AUDIO"},
			SpeechConfig: &genai.SpeechConfig{
				LanguageCode: language,
				VoiceConfig: &genai.VoiceConfig{
					PrebuiltVoiceConfig: &genai.PrebuiltVoiceConfig{
						VoiceName: voice, // Puck, Charon, Kore, Fenrir, Aoede
					},
				},
			},
		},
	}
}

// Close implements [io.WriteCloser].
func (a *AiStudioTts) Close() error { return a.w.Close() }

// Write implements [io.WriteCloser].
func (a *AiStudioTts) Write(p []byte) (n int, err error) {
	for {
		resp, err := a.client.Models.GenerateContent(
			a.ctx, a.model,
			[]*genai.Content{{Parts: []*genai.Part{{Text: a.prompt + " " + string(p)}}}},
			&a.config)
		if err != nil {
			log.Printf("Error generating TTS content: %v, retrying\n", err)
			continue
		}
		var rawAudioData []byte
		for _, cand := range resp.Candidates {
			if cand.Content != nil {
				for _, part := range cand.Content.Parts {
					if part.InlineData != nil {
						rawAudioData = part.InlineData.Data
					}
				}
			}
		}
		if len(rawAudioData) == 0 {
			log.Printf("No audio data returned from the model. Retrying\n")
		} else {
			wavAudio := encodeWav(rawAudioData, 24000)
			mp3, err := ConvertWavToMp3(wavAudio)
			if err != nil {
				return 0, err
			}
			return a.w.Write(mp3)
		}
	}
}

func encodeWav(pcmData []byte, sampleRate uint32) []byte {
	var buf bytes.Buffer

	numChannels := uint16(1)    // Mono
	bitsPerSample := uint16(16) // 16-bit

	subChunk2Size := uint32(len(pcmData))
	chunkSize := 36 + subChunk2Size
	byteRate := sampleRate * uint32(numChannels) * uint32(bitsPerSample) / 8
	blockAlign := numChannels * bitsPerSample / 8

	// --- 1. RIFF Header ---
	buf.Write([]byte("RIFF"))
	_ = binary.Write(&buf, binary.LittleEndian, chunkSize)
	buf.Write([]byte("WAVE"))

	// --- 2. fmt Sub-chunk ---
	buf.Write([]byte("fmt "))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16)) // Subchunk1Size (16 for PCM)
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))  // AudioFormat (1 for PCM / uncompressed)
	_ = binary.Write(&buf, binary.LittleEndian, numChannels)
	_ = binary.Write(&buf, binary.LittleEndian, sampleRate)
	_ = binary.Write(&buf, binary.LittleEndian, byteRate)
	_ = binary.Write(&buf, binary.LittleEndian, blockAlign)
	_ = binary.Write(&buf, binary.LittleEndian, bitsPerSample)

	// --- 3. data Sub-chunk ---
	buf.Write([]byte("data"))
	_ = binary.Write(&buf, binary.LittleEndian, subChunk2Size)
	buf.Write(pcmData)

	// Write the buffer to file
	return buf.Bytes()
}
