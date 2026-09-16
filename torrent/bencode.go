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

/*
data:
i 1 2 3 e
↑
pos = 0
*/
type BencodeDecoder struct {
	data []byte /// the complete Bencode data
	pos  int    /// where we currently are
}

func DecodeBencode(data []byte) (any, error) {
	/// Initially pos is 0
	d := &BencodeDecoder{
		data: data,
	}

	/// The decoder looks at the current byte
	/// and figures out what type it is
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

	/// Parse the integer
	value, err := strconv.ParseInt(
		string(d.data[start:d.pos]),
		10,
		64,
	)

	if err != nil {
		return 0, fmt.Errorf("invalid integer: %w", err)
	}

	/// Moves past e
	d.pos++

	return value, nil
}

func (d *BencodeDecoder) decodeString() ([]byte, error) {
	/*
		For string the format is:

		[length]:[data]

		4:spam
		↑
		pos = 0
	*/
	start := d.pos

	for d.pos < len(d.data) && d.data[d.pos] != ':' {
		d.pos++
	}

	if d.pos >= len(d.data) {
		return nil, fmt.Errorf("invalid string length")
	}

	/// Get the length of the string
	length, err := strconv.Atoi(
		string(d.data[start:d.pos]),
	)

	if err != nil {
		return nil, fmt.Errorf("invalid string length: %w", err)
	}

	/// Move past `:`
	/// 4:spam
	///   ↑
	d.pos++

	/*
		Ex: 4:spam
			  ↑
		So end = 2 + 4 = 6
	*/
	end := d.pos + length

	if end > len(d.data) {
		return nil, fmt.Errorf("string exceeds input")
	}

	/*
		create a new byte slice containing the string.

		Bencode i/p 4:spam -> []byte("spam")

		This is useful in torrent files because some Bencode
		strings are binary data not normal text
	*/
	value := make([]byte, length)

	/// Copy the string to the value
	copy(value, d.data[d.pos:end])

	d.pos = end

	return value, nil
}

func (d *BencodeDecoder) decodeList() ([]any, error) {
	/// Move past `l`
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

		/// Decode each value inside the list
		value, err := d.decode()
		if err != nil {
			return nil, err
		}

		/// Append the value to the result
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

		/// Dict key in bencode is always a string
		keyRaw, err := d.decodeString()
		if err != nil {
			return nil, err
		}

		/// Decode the value
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
		/*
			Why sort the keys?

			Dictionary keys must be sorted.
			This ensures deterministic Bencode,
			which is essential when calculating
			the torrent's info hash.

			Suppose:

			info = {
				"name": "test.txt",
				"length": 100
			}

			It must be encoded deterministically.

			If one encoding produced:

			d6:lengthi100e4:name8:test.txte

			while another produced:

			d4:name8:test.txt6:lengthi100ee

			they represent the same logical map, but their SHA-1 hashes would be different.

			BitTorrent needs everyone to calculate the same info hash.
		*/
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
