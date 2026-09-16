package tracker

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"time"
)

/*This adds UDP tracker support.*/

/*
 * Endianness Demo: Storing Data 0x12345678 (4 Bytes)
 * --------------------------------------------------
 * Byte 3: 0x12 (MSB - Most Significant Byte)
 * Byte 2: 0x34
 * Byte 1: 0x56
 * Byte 0: 0x78 (LSB - Least Significant Byte)
 *
 * LITTLE ENDIAN (LSB First / Backwards):
 * - Address 0x0 : 0x78 (Byte 0)
 * - Address 0x1 : 0x56 (Byte 1)
 * - Address 0x2 : 0x34 (Byte 2)
 * - Address 0x3 : 0x12 (Byte 3)
 *
 * BIG ENDIAN (MSB First / Human-Readable):
 * - Address 0x0 : 0x12 (Byte 3)
 * - Address 0x1 : 0x34 (Byte 2)
 * - Address 0x2 : 0x56 (Byte 1)
 * - Address 0x3 : 0x78 (Byte 0)
 */

const (
	// identifies BitTorrent UDP protocol
	protocolID uint64 = 0x41727101980
	// 0 means CONNECT
	// 1 means ANNOUNCE
	actionConnect  uint32 = 0
	actionAnnounce uint32 = 1
)

func AnnounceUDP(
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

	addr, err := net.ResolveUDPAddr(
		"udp",
		u.Host,
	)

	if err != nil {
		return nil, err
	}

	conn, err := net.DialUDP(
		"udp",
		nil,
		addr,
	)

	if err != nil {
		return nil, err
	}

	defer conn.Close()

	conn.SetDeadline(
		time.Now().Add(10 * time.Second),
	)

	transactionID := rand.Uint32()

	/*
		Connect Packet:
		┌───────────────┬──────────┬───────────────┐
		│ Protocol ID   │ Action   │ Transaction ID│
		│   8 bytes     │ 4 bytes  │    4 bytes    │
		└───────────────┴──────────┴───────────────┘
	*/
	connectPacket := make([]byte, 16)

	binary.BigEndian.PutUint64(
		connectPacket[0:8],
		protocolID,
	)

	binary.BigEndian.PutUint32(
		connectPacket[8:12],
		actionConnect,
	)

	binary.BigEndian.PutUint32(
		connectPacket[12:16],
		transactionID,
	)

	// Client ─── CONNECT ───> Tracker
	if _, err := conn.Write(connectPacket); err != nil {
		return nil, err
	}

	response := make([]byte, 2048)

	/*
		Tracker responds with something containing:
		action
		transaction ID
		connection ID
	*/
	n, err := conn.Read(response)
	if err != nil {
		return nil, err
	}

	if n < 16 {
		return nil, fmt.Errorf("short UDP tracker response")
	}

	action := binary.BigEndian.Uint32(response[0:4])

	if action != actionConnect {
		return nil, fmt.Errorf("unexpected connect action")
	}

	/// connectionID as a temporary token from the tracker that we need for the next request.
	connectionID := binary.BigEndian.Uint64(
		response[8:16],
	)

	transactionID = rand.Uint32()

	/*
		Create announce packet
		announce := make([]byte, 98)

		The UDP announce request has 98 bytes.

		Conceptually:

		┌──────────────────────┐
		│ Connection ID  8     │
		│ Action        4      │
		│ Transaction ID 4     │
		│ Info Hash     20     │
		│ Peer ID       20     │
		│ Downloaded    8      │
		│ Left          8      │
		│ Uploaded      8      │
		│ Event         4      │
		│ IP            4      │
		│ Key           4      │
		│ Num Want      4      │
		│ Port          2      │
		└──────────────────────┘
		98 bytes
	*/
	announce := make([]byte, 98)

	binary.BigEndian.PutUint64(
		announce[0:8],
		connectionID,
	)

	binary.BigEndian.PutUint32(
		announce[8:12],
		actionAnnounce,
	)

	binary.BigEndian.PutUint32(
		announce[12:16],
		transactionID,
	)

	/// infoHash[:] means I want peers for THIS torrent.
	copy(announce[16:36], infoHash[:])
	/// My peer ID is this
	copy(announce[36:56], peerID[:])

	binary.BigEndian.PutUint64(
		announce[56:64],
		uint64(0),
	) /// downloaded status

	binary.BigEndian.PutUint64(
		announce[64:72],
		uint64(left),
	) /// remaining status

	binary.BigEndian.PutUint64(
		announce[72:80],
		uint64(0),
	) /// uploaded status

	/// event
	// 0: None
	// 1: Started
	// 2: Completed
	// 3: Stopped
	binary.BigEndian.PutUint32(
		announce[80:84],
		0,
	)

	/// ip
	/// 0 would mean:
	/// "Tracker, determine my IP address.
	binary.BigEndian.PutUint32(
		announce[84:88],
		0,
	)

	/*
		num_want.

		It tells the tracker:

		"How many peers do I want?"

		0xffffffff means:

		-1 as a signed 32-bit value

		which means:

		Use the tracker's default number of peers.
	*/
	binary.BigEndian.PutUint32(
		announce[88:92],
		0xffffffff,
	)

	binary.BigEndian.PutUint16(
		announce[96:98],
		port,
	)

	/// Client ─── ANNOUNCE ───> Tracker
	if _, err := conn.Write(announce); err != nil {
		return nil, err
	}

	n, err = conn.Read(response)
	if err != nil {
		return nil, err
	}

	if n < 20 {
		return nil, fmt.Errorf("short announce response")
	}

	action = binary.BigEndian.Uint32(response[0:4])

	if action != actionAnnounce {
		return nil, fmt.Errorf("unexpected announce action")
	}

	/// The first 20 bytes are response metadata; the remaining bytes contain peers.
	peerBytes := response[20:n]

	/*
		 each compact IPv4 peer uses 6 bytes:

		4 bytes → IP
		2 bytes → Port
	*/
	if len(peerBytes)%6 != 0 {
		return nil, fmt.Errorf("invalid UDP peer list")
	}

	return parseCompactPeers(peerBytes)
}
