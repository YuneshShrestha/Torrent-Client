package torrent

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
)

/*
	BitTorrent uses four basic Bencode types:

	integer → i123e
	string  → 4:spam
	list    → l...e
	dict    → d...e

*/

type BencodeDecoder struct {
	data []byte
	pos  int
}

func DecodeBencode(data []byte) (any, error) {
	d := &BencodeDecoder{
		data: data,
	}

	value, err := d.decode()
	if err != nil {
		return nil, err
	}

	if d.pos != len(d.data) {
		return nil, fmt.Errorf("trailing data at position %d", d.pos)
	}

	return value, nil
}

func (d *BencodeDecoder) decode() (any, error) {
	if d.pos >= len(d.data) {
		return nil, fmt.Errorf("unexpected end of data")
	}

	switch d.data[d.pos] {
	case 'i':
		return d.decodeInt()

	case 'l':
		return d.decodeList()

	case 'd':
		return d.decodeDict()

	default:
		if d.data[d.pos] >= '0' && d.data[d.pos] <= '9' {
			return d.decodeString()
		}

		return nil, fmt.Errorf(
			"invalid bencode character %q at position %d",
			d.data[d.pos],
			d.pos,
		)
	}
}

func (d *BencodeDecoder) decodeInt() (int64, error) {
	d.pos++

	start := d.pos

	for d.pos < len(d.data) && d.data[d.pos] != 'e' {
		d.pos++
	}

	if d.pos >= len(d.data) {
		return 0, fmt.Errorf("unterminated integer")
	}

	value, err := strconv.ParseInt(
		string(d.data[start:d.pos]),
		10,
		64,
	)

	if err != nil {
		return 0, fmt.Errorf("invalid integer: %w", err)
	}

	d.pos++

	return value, nil
}

func (d *BencodeDecoder) decodeString() ([]byte, error) {
	start := d.pos

	for d.pos < len(d.data) && d.data[d.pos] != ':' {
		d.pos++
	}

	if d.pos >= len(d.data) {
		return nil, fmt.Errorf("invalid string length")
	}

	length, err := strconv.Atoi(
		string(d.data[start:d.pos]),
	)

	if err != nil {
		return nil, fmt.Errorf("invalid string length: %w", err)
	}

	d.pos++

	end := d.pos + length

	if end > len(d.data) {
		return nil, fmt.Errorf("string exceeds input")
	}

	value := make([]byte, length)
	copy(value, d.data[d.pos:end])

	d.pos = end

	return value, nil
}

func (d *BencodeDecoder) decodeList() ([]any, error) {
	d.pos++

	var result []any

	for {
		if d.pos >= len(d.data) {
			return nil, fmt.Errorf("unterminated list")
		}

		if d.data[d.pos] == 'e' {
			d.pos++
			return result, nil
		}

		value, err := d.decode()
		if err != nil {
			return nil, err
		}

		result = append(result, value)
	}
}

func (d *BencodeDecoder) decodeDict() (map[string]any, error) {
	d.pos++

	result := make(map[string]any)

	for {
		if d.pos >= len(d.data) {
			return nil, fmt.Errorf("unterminated dictionary")
		}

		if d.data[d.pos] == 'e' {
			d.pos++
			return result, nil
		}

		keyRaw, err := d.decodeString()
		if err != nil {
			return nil, err
		}

		value, err := d.decode()
		if err != nil {
			return nil, err
		}

		result[string(keyRaw)] = value
	}
}

func encodeBencode(value any) ([]byte, error) {
	var buf bytes.Buffer

	if err := encodeValue(&buf, value); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func encodeValue(buf *bytes.Buffer, value any) error {
	switch v := value.(type) {

	case int64:
		buf.WriteByte('i')
		buf.WriteString(strconv.FormatInt(v, 10))
		buf.WriteByte('e')

	case []byte:
		buf.WriteString(strconv.Itoa(len(v)))
		buf.WriteByte(':')
		buf.Write(v)

	case string:
		buf.WriteString(strconv.Itoa(len(v)))
		buf.WriteByte(':')
		buf.WriteString(v)

	case []any:
		buf.WriteByte('l')

		for _, item := range v {
			if err := encodeValue(buf, item); err != nil {
				return err
			}
		}

		buf.WriteByte('e')

	case map[string]any:
		buf.WriteByte('d')

		keys := make([]string, 0, len(v))

		for key := range v {
			keys = append(keys, key)
		}

		sort.Strings(keys)

		for _, key := range keys {
			if err := encodeValue(buf, []byte(key)); err != nil {
				return err
			}

			if err := encodeValue(buf, v[key]); err != nil {
				return err
			}
		}

		buf.WriteByte('e')

	default:
		return fmt.Errorf("cannot bencode %T", value)
	}

	return nil
}
