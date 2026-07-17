package converter

import (
	"bytes"
	"fmt"

	"github.com/braheezy/shine-mp3/pkg/mp3"
	"github.com/go-audio/wav"
)

func ConvertWavToMp3(wavData []byte) ([]byte, error) {
	// Read the incoming WAV bytes container
	// Decode WAV file
	decoder := wav.NewDecoder(bytes.NewReader(wavData))
	wavBuffer, err := decoder.FullPCMBuffer()
	if err != nil {
		return nil, fmt.Errorf("Error decoding WAV: %v", err)
	}
	if !decoder.IsValidFile() {
		return nil, fmt.Errorf("invalid wav payload returned from API")
	}
	// Convert go-audio IntBuffer data to []int16 slices expected by the MP3 encoder
	pcmSamples := make([]int16, len(wavBuffer.Data))
	for i, val := range wavBuffer.Data {
		pcmSamples[i] = int16(val)
	}
	encoder := mp3.NewEncoder(int(decoder.SampleRate), int(decoder.NumChans))
	if encoder == nil {
		return nil, fmt.Errorf("failed to initialize shine mp3 encoder")
	}
	var out bytes.Buffer
	err = encoder.Write(&out, pcmSamples)
	if err != nil {
		return nil, fmt.Errorf("Error encoding MP3: %v", err)
	}
	return out.Bytes(), nil
}
