package seeder

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

type offsetReader struct {
	r       io.Reader
	pos     int64
	lastLog time.Time
}

func (o *offsetReader) Read(p []byte) (int, error) {
	n, err := o.r.Read(p)
	if n > 0 {
		o.pos += int64(n)
		if time.Since(o.lastLog) >= 5*time.Second {
			log.Printf("  JSON layout scan: %.2f GB read...", float64(o.pos)/1e9)
			o.lastLog = time.Now()
		}
	}
	return n, err
}

func skipValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}

	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}

	switch delim {
	case '{':
		for dec.More() {
			if err := skipValue(dec); err != nil {
				return err
			}
			if err := skipValue(dec); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	case '[':
		for dec.More() {
			if err := skipValue(dec); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	default:
		return fmt.Errorf("unexpected delimiter %v", delim)
	}
}

// buildSectionIndex scans the top-level JSON object once and records the byte offset
// inside each top-level array (immediately after the opening '[').
func buildSectionIndex(jsonPath string) (map[string]int64, error) {
	f, err := os.Open(jsonPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	or := &offsetReader{r: f, lastLog: time.Now()}
	dec := json.NewDecoder(or)

	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("read JSON start: %w", err)
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, fmt.Errorf("expected top-level JSON object")
	}

	index := make(map[string]int64, 24)
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("read section key: %w", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("expected string key at top level")
		}

		first, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("read value for %q: %w", key, err)
		}
		valueDelim, ok := first.(json.Delim)
		if !ok || valueDelim != '[' {
			if err := skipValueFrom(dec, first); err != nil {
				return nil, err
			}
			continue
		}
		// Offset after '[' — decoder at first array element (or closing ']').
		index[key] = or.pos
		for dec.More() {
			if err := skipValue(dec); err != nil {
				return nil, err
			}
		}
		if _, err := dec.Token(); err != nil {
			return nil, fmt.Errorf("close array for %q: %w", key, err)
		}
	}
	return index, nil
}

func skipValueFrom(dec *json.Decoder, first json.Token) error {
	delim, ok := first.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		for dec.More() {
			if err := skipValue(dec); err != nil {
				return err
			}
			if err := skipValue(dec); err != nil {
				return err
			}
		}
		_, err := dec.Token()
		return err
	case '[':
		for dec.More() {
			if err := skipValue(dec); err != nil {
				return err
			}
		}
		_, err := dec.Token()
		return err
	default:
		return fmt.Errorf("unexpected delimiter %v", delim)
	}
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

	dec := json.NewDecoder(f)
	return f, dec, nil
}
