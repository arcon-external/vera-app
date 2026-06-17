package backend

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

type Sequence struct {
	Next uint16 `json:"next"`
}

func getNextSequence() (uint16, error) {
	seq, err := readSequence()
	if err != nil {
		return 0, fmt.Errorf("read sequence: %w", err)
	}

	if seq.Next > 9999 {
		return 0, fmt.Errorf("sequence has reached its maximum value")
	}

	nextSeq := seq.Next
	seq.Next++

	if err = writeSequence(seq); err != nil {
		return 0, fmt.Errorf("write sequence: %w", err)
	}

	return nextSeq, nil
}

func readSequence() (*Sequence, error) {
	var seq *Sequence

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get user home dir failed: %w", err)
	}

	sequenceDir := homeDir + "/.vera-quote/"
	sequencePath := sequenceDir + "sequence.json"

	if err = os.MkdirAll(sequenceDir, 0o755); err != nil {
		return nil, fmt.Errorf("create sequence dir failed: %w", err)
	}

	seqFile, err := os.OpenFile(sequencePath, os.O_CREATE|os.O_RDONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open sequence file failed: %w", err)
	}
	defer seqFile.Close()

	jsonParser := json.NewDecoder(seqFile)
	if err = jsonParser.Decode(&seq); err != nil {
		if errors.Is(err, io.EOF) {
			seq = &Sequence{
				Next: 1,
			}
		} else {
			return nil, fmt.Errorf("decoding file: %w", err)
		}
	}

	return seq, nil
}

func writeSequence(seq *Sequence) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get user home dir failed: %w", err)
	}

	sequenceDir := homeDir + "/.vera-quote/"
	sequencePath := sequenceDir + "sequence.json"

	if err = os.MkdirAll(sequenceDir, 0o755); err != nil {
		return fmt.Errorf("create sequence dir failed: %w", err)
	}

	seqFile, err := os.OpenFile(sequencePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open sequence file failed: %w", err)
	}
	defer seqFile.Close()

	jsonEncoder := json.NewEncoder(seqFile)
	if err = jsonEncoder.Encode(seq); err != nil {
		return fmt.Errorf("encode sequence to file: %w", err)
	}

	return nil
}
