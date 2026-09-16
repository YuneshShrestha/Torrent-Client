package tracker

/*This communicates with HTTP trackers.*/
/*
AnnounceHTTP()
      │
      ├── Parse tracker URL
      │
      ├── Add announce parameters
      │      │
      │      ├── info_hash
      │      ├── peer_id
      │      ├── port
      │      ├── uploaded
      │      ├── downloaded
      │      ├── left
      │      ├── compact=1
      │      └── event=started
      │
      ├── HTTP GET
      │
      ▼
   TRACKER
      │
      │ bencoded response
      ▼
   decode()
      │
      ├── integer()
      ├── list()
      ├── dict()
      └── string()
      │
      ▼
map[string]any
      │
      ├── "interval" → int64
      │
      └── "peers" → []byte
                       │
                       ▼
              parseCompactPeers()
                       │
              ┌────────┴────────┐
              ▼                 ▼
        6 bytes/peer       6 bytes/peer
        ┌─────────┐        ┌─────────┐
        │ 4 IP    │        │ 4 IP    │
        │ 2 Port  │        │ 2 Port  │
        └─────────┘        └─────────┘
              │                 │
              ▼                 ▼
       Peer{IP, Port}     Peer{IP, Port}
              │                 │
              └────────┬────────┘
                       ▼
                    []Peer

*/
import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Peer struct {
	IP   string
	Port uint16
}

// sends an announce request to the tracker and returns a list of peers
/*
trackerURL = http://tracker.example.com/announce
infoHash   = 20-byte torrent identifier
peerID     = 20-byte identifier for your client
port       = 6881
left       = bytes remaining to download
*/
func AnnounceHTTP(
	trackerURL string,
	infoHash [20]byte,
	peerID [20]byte,
	port uint16,
	left int64,
) ([]Peer, error) {

	/// Go creates a URL structure.
	u, err := url.Parse(trackerURL)
	if err != nil {
		return nil, err
	}

	/// gets the query parameters.
	query := u.Query()

	/*
		Conceptually, you're building:

		http://tracker.example.com/announce?
			info_hash=...
			&peer_id=...
			&port=6881
			&uploaded=0
			&downloaded=0
			&left=123456
			&compact=1
			&event=started

		info_hash and peer_id are 20 raw bytes, not normal text.
		query.Encode() URL-encodes those bytes so they can safely travel in the HTTP URL.
	*/
	query.Set("info_hash", string(infoHash[:]))
	query.Set("peer_id", string(peerID[:]))
	query.Set("port", strconv.Itoa(int(port)))
	query.Set("uploaded", "0")
	query.Set("downloaded", "0")
	query.Set("left", strconv.FormatInt(left, 10)) /// 10 is the base, converts an int64 number into a decimal string.
	query.Set("compact", "1")
	query.Set("event", "started")

	u.RawQuery = query.Encode()

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	/// sends a GET request to the tracker.
	/*
		u.String()
		might produce something conceptually like:
		http://tracker.example.com/announce?compact=1&downloaded=0&event=started&info_hash=%A3%...&left=123456&peer_id=%2D...&port=6881&uploaded=0
	*/
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

	/*
		Suppose the tracker returns:
		d
		8:interval
		i1800e
		5:peers
		12:<12 binary bytes>
		e

		Remember that the peers data is binary, so it won't necessarily look readable when printed.

	*/
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	/*
		decodeTrackerResponse decodes the tracker response.

			Your tracker response:
			d
			8:interval
			i1800e
			5:peers
			12:<binary data>
			e

			becomes conceptually:

			map[string]any{
				"interval": int64(1800),

				"peers": []byte{
					// binary peer data
				},
			}

	*/
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

	/// Get compact peers
	/*
		Example compact peer

		Suppose:

		data = []byte{
			192,
			168,
			1,
			10,
			0x1A,
			0xE1,
		}

		That's:

		192       → IP byte 1
		168       → IP byte 2
		1         → IP byte 3
		10        → IP byte 4

		0x1A      → port byte 1
		0xE1      → port byte 2

		So:
		192.168.1.10:6881

	*/
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
	/*
		Every IPv4 peer requires exactly:

		4 bytes IP
		+
		2 bytes port
		=
		6 bytes

		So if you have:

		6 bytes  → 1 peer
		12 bytes → 2 peers
		18 bytes → 3 peers
		24 bytes → 4 peers
	*/
	if len(data)%6 != 0 {
		return nil, fmt.Errorf("invalid compact peer list")
	}

	var peers []Peer
	/*
		data = [192, 168, 1, 10, 0x1A, 0xE1]
		IP = 192.168.1.10
	*/

	for i := 0; i < len(data); i += 6 {
		ip := fmt.Sprintf(
			"%d.%d.%d.%d",
			data[i],
			data[i+1],
			data[i+2],
			data[i+3],
		)

		/*
			The last 2 bytes contain the port in big-endian format.

			Example:

			0x1A E1

			0x1A << 8 = 6656
			0xE1      = 225

			6656 + 225 = 6881
			So:

			Port = 6881
		*/
		port := uint16(data[i+4])<<8 |
			uint16(data[i+5])

		peers = append(peers, Peer{
			IP:   ip,
			Port: port,
		})
	}

	return peers, nil
}
