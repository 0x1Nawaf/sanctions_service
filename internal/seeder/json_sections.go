package seeder

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

// buildSectionIndex scans the top-level JSON object once using a byte scanner (no
// parsing of array/object contents). For each top-level array it stores the file
// offset immediately after the opening '['.
func buildSectionIndex(jsonPath string) (map[string]int64, error) {
	f, err := os.Open(jsonPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	totalSize := info.Size()
	lastLog := time.Now()

	r := newByteOffsetReader(f)
	if err := r.expectByte('{'); err != nil {
		return nil, fmt.Errorf("read JSON start: %w", err)
	}

	index := make(map[string]int64, 24)
	firstKey := true
	for {
		if time.Since(lastLog) >= 5*time.Second {
			var pct float64
			if totalSize > 0 {
				pct = float64(r.pos) / float64(totalSize) * 100
			}
			log.Printf("  JSON layout scan: %.2f GB / %.2f GB (%.0f%%)...",
				float64(r.pos)/1e9, float64(totalSize)/1e9, pct)
			lastLog = time.Now()
		}

		if err := r.skipWhitespace(); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		b, err := r.ReadByte()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if b == '}' {
			break
		}
		if b == ',' {
			if firstKey {
				return nil, fmt.Errorf("unexpected comma at offset %d", r.pos)
			}
			if err := r.skipWhitespace(); err != nil {
				return nil, err
			}
			b, err = r.ReadByte()
			if err != nil {
				return nil, err
			}
		}
		if b != '"' {
			return nil, fmt.Errorf("expected key string at offset %d", r.pos)
		}
		if err := r.UnreadByte(b); err != nil {
			return nil, err
		}

		key, err := r.readJSONString()
		if err != nil {
			return nil, fmt.Errorf("read key: %w", err)
		}
		if err := r.expectByte(':'); err != nil {
			return nil, fmt.Errorf("after key %q: %w", key, err)
		}
		if err := r.skipWhitespace(); err != nil {
			return nil, err
		}
		val, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("read value for %q: %w", key, err)
		}
		if val == '[' {
			index[key] = r.pos
			if err := r.skipBalanced('[', ']'); err != nil {
				return nil, fmt.Errorf("skip array %q: %w", key, err)
			}
		} else {
			if err := r.skipJSONValue(val); err != nil {
				return nil, fmt.Errorf("skip value %q: %w", key, err)
			}
		}
		firstKey = false
	}
	return index, nil
}

func openSectionDecoder(jsonPath, key string, index map[string]int64) (*os.File, *json.Decoder, error) {
	off, ok := index[key]
	if !ok {
		return nil, nil, fmt.Errorf("key %q not found", key)
	}

	f, err := os.Open(jsonPath)
	if err != nil {
		return nil, nil, err
	}

	if _, err := f.Seek(off, io.SeekStart); err != nil {
		f.Close()
		return nil, nil, err
	}

	// File offset is inside the array (first element or ']'). Prefix '[' so the
	// decoder sees a valid JSON array and dec.More() works.
	dec := json.NewDecoder(io.MultiReader(strings.NewReader("["), f))
	tok, err := dec.Token()
	if err != nil {
		f.Close()
		return nil, nil, fmt.Errorf("read array for %q: %w", key, err)
	}
	d, ok := tok.(json.Delim)
	if !ok || d != '[' {
		f.Close()
		return nil, nil, fmt.Errorf("expected array for key %q", key)
	}
	return f, dec, nil
}
