package tracker

/*This communicates with HTTP trackers.*/
import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Peer struct {
	IP   string
	Port uint16
}

func AnnounceHTTP(
	trackerURL string,
	infoHash [20]byte,
	peerID [20]byte,
	port uint16,
	left int64,
) ([]Peer, error) {

	u, err := url.Parse(trackerURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()

	query.Set("info_hash", string(infoHash[:]))
	query.Set("peer_id", string(peerID[:]))
	query.Set("port", strconv.Itoa(int(port)))
	query.Set("uploaded", "0")
	query.Set("downloaded", "0")
	query.Set("left", strconv.FormatInt(left, 10))
	query.Set("compact", "1")
	query.Set("event", "started")

	u.RawQuery = query.Encode()

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	resp, err := client.Get(u.String())
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"tracker returned HTTP %d",
			resp.StatusCode,
		)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	value, err := decodeTrackerResponse(body)
	if err != nil {
		return nil, err
	}

	dict, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid tracker response")
	}

	if failure, ok := dict["failure reason"].([]byte); ok {
		return nil, fmt.Errorf(
			"tracker failure: %s",
			string(failure),
		)
	}

	peersRaw, ok := dict["peers"].([]byte)
	if !ok {
		return nil, fmt.Errorf("tracker did not return compact peers")
	}

	return parseCompactPeers(peersRaw)
}

func decodeTrackerResponse(data []byte) (any, error) {
	return decode(data)
}

type decoder struct {
	data []byte
	pos  int
}

func decode(data []byte) (any, error) {
	d := &decoder{data: data}
	return d.value()
}

func (d *decoder) value() (any, error) {
	if d.pos >= len(d.data) {
		return nil, fmt.Errorf("unexpected end")
	}

	switch d.data[d.pos] {
	case 'i':
		return d.integer()

	case 'l':
		return d.list()

	case 'd':
		return d.dict()

	default:
		return d.string()
	}
}

func (d *decoder) integer() (int64, error) {
	d.pos++

	start := d.pos

	for d.pos < len(d.data) && d.data[d.pos] != 'e' {
		d.pos++
	}

	if d.pos >= len(d.data) {
		return 0, fmt.Errorf("invalid integer")
	}

	n, err := strconv.ParseInt(
		string(d.data[start:d.pos]),
		10,
		64,
	)

	d.pos++

	return n, err
}

func (d *decoder) string() ([]byte, error) {
	start := d.pos

	for d.pos < len(d.data) && d.data[d.pos] != ':' {
		d.pos++
	}

	if d.pos >= len(d.data) {
		return nil, fmt.Errorf("invalid string")
	}

	length, err := strconv.Atoi(
		string(d.data[start:d.pos]),
	)

	if err != nil {
		return nil, err
	}

	d.pos++

	end := d.pos + length

	if end > len(d.data) {
		return nil, fmt.Errorf("string out of bounds")
	}

	result := make([]byte, length)

	copy(result, d.data[d.pos:end])

	d.pos = end

	return result, nil
}

func (d *decoder) list() ([]any, error) {
	d.pos++

	var result []any

	for {
		if d.data[d.pos] == 'e' {
			d.pos++
			return result, nil
		}

		value, err := d.value()
		if err != nil {
			return nil, err
		}

		result = append(result, value)
	}
}

func (d *decoder) dict() (map[string]any, error) {
	d.pos++

	result := make(map[string]any)

	for {
		if d.data[d.pos] == 'e' {
			d.pos++
			return result, nil
		}

		keyRaw, err := d.string()
		if err != nil {
			return nil, err
		}

		value, err := d.value()
		if err != nil {
			return nil, err
		}

		result[string(keyRaw)] = value
	}
}

func parseCompactPeers(data []byte) ([]Peer, error) {
	if len(data)%6 != 0 {
		return nil, fmt.Errorf("invalid compact peer list")
	}

	var peers []Peer

	for i := 0; i < len(data); i += 6 {
		ip := fmt.Sprintf(
			"%d.%d.%d.%d",
			data[i],
			data[i+1],
			data[i+2],
			data[i+3],
		)

		port := uint16(data[i+4])<<8 |
			uint16(data[i+5])

		peers = append(peers, Peer{
			IP:   ip,
			Port: port,
		})
	}

	return peers, nil
}

func normalizeTrackerURL(raw string) string {
	return strings.TrimSpace(raw)
}
