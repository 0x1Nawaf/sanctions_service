package seeder

import (
	"bufio"
	"fmt"
	"os"
)

type byteOffsetReader struct {
	br  *bufio.Reader
	pos int64
}

func newByteOffsetReader(f *os.File) *byteOffsetReader {
	return &byteOffsetReader{br: bufio.NewReaderSize(f, 256*1024)}
}

func (r *byteOffsetReader) ReadByte() (byte, error) {
	b, err := r.br.ReadByte()
	if err == nil {
		r.pos++
	}
	return b, err
}

func (r *byteOffsetReader) UnreadByte(b byte) error {
	if err := r.br.UnreadByte(); err != nil {
		return err
	}
	r.pos--
	return nil
}

func (r *byteOffsetReader) skipWhitespace() error {
	for {
		b, err := r.ReadByte()
		if err != nil {
			return err
		}
		if b > ' ' {
			return r.UnreadByte(b)
		}
	}
}

func (r *byteOffsetReader) expectByte(want byte) error {
	if err := r.skipWhitespace(); err != nil {
		return err
	}
	b, err := r.ReadByte()
	if err != nil {
		return err
	}
	if b != want {
		return fmt.Errorf("expected %q, got %q at offset %d", want, b, r.pos)
	}
	return nil
}

func (r *byteOffsetReader) readJSONString() (string, error) {
	if err := r.skipWhitespace(); err != nil {
		return "", err
	}
	b, err := r.ReadByte()
	if err != nil {
		return "", err
	}
	if b != '"' {
		return "", fmt.Errorf("expected string at offset %d", r.pos)
	}

	var buf []byte
	inEscape := false
	for {
		b, err = r.ReadByte()
		if err != nil {
			return "", err
		}
		if inEscape {
			buf = append(buf, b)
			inEscape = false
			continue
		}
		if b == '\\' {
			inEscape = true
			continue
		}
		if b == '"' {
			return string(buf), nil
		}
		buf = append(buf, b)
	}
}

func (r *byteOffsetReader) skipJSONStringAfterOpen() error {
	inEscape := false
	for {
		b, err := r.ReadByte()
		if err != nil {
			return err
		}
		if inEscape {
			inEscape = false
			continue
		}
		if b == '\\' {
			inEscape = true
			continue
		}
		if b == '"' {
			return nil
		}
	}
}

func (r *byteOffsetReader) expectLiteral(lit string) error {
	for i := 1; i < len(lit); i++ {
		b, err := r.ReadByte()
		if err != nil {
			return err
		}
		if b != lit[i] {
			return fmt.Errorf("expected %q at offset %d", lit, r.pos)
		}
	}
	return nil
}

func (r *byteOffsetReader) skipBalanced(open, close byte) error {
	depth := 1
	inString := false
	inEscape := false
	for depth > 0 {
		b, err := r.ReadByte()
		if err != nil {
			return err
		}
		if inString {
			if inEscape {
				inEscape = false
				continue
			}
			if b == '\\' {
				inEscape = true
				continue
			}
			if b == '"' {
				inString = false
			}
			continue
		}
		switch b {
		case '"':
			inString = true
		case open:
			depth++
		case close:
			depth--
		}
	}
	return nil
}

func (r *byteOffsetReader) skipJSONValue(first byte) error {
	switch first {
	case '{':
		return r.skipBalanced('{', '}')
	case '[':
		return r.skipBalanced('[', ']')
	case '"':
		return r.skipJSONStringAfterOpen()
	case 't':
		if err := r.UnreadByte(first); err != nil {
			return err
		}
		return r.expectLiteral("true")
	case 'f':
		if err := r.UnreadByte(first); err != nil {
			return err
		}
		return r.expectLiteral("false")
	case 'n':
		if err := r.UnreadByte(first); err != nil {
			return err
		}
		return r.expectLiteral("null")
	default:
		b := first
		for {
			if (b >= '0' && b <= '9') || b == '-' || b == '+' || b == '.' ||
				b == 'e' || b == 'E' {
				var err error
				b, err = r.ReadByte()
				if err != nil {
					return err
				}
				continue
			}
			return r.UnreadByte(b)
		}
	}
}
