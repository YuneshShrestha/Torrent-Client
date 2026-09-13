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

const (
	protocolID     uint64 = 0x41727101980
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

	if _, err := conn.Write(connectPacket); err != nil {
		return nil, err
	}

	response := make([]byte, 2048)

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

	connectionID := binary.BigEndian.Uint64(
		response[8:16],
	)

	transactionID = rand.Uint32()

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

	copy(announce[16:36], infoHash[:])
	copy(announce[36:56], peerID[:])

	binary.BigEndian.PutUint64(
		announce[56:64],
		uint64(0),
	)

	binary.BigEndian.PutUint64(
		announce[64:72],
		uint64(left),
	)

	binary.BigEndian.PutUint64(
		announce[72:80],
		uint64(0),
	)

	binary.BigEndian.PutUint32(
		announce[80:84],
		0,
	)

	binary.BigEndian.PutUint32(
		announce[84:88],
		rand.Uint32(),
	)

	binary.BigEndian.PutUint32(
		announce[88:92],
		0xffffffff,
	)

	binary.BigEndian.PutUint16(
		announce[96:98],
		port,
	)

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

	peerBytes := response[20:n]

	if len(peerBytes)%6 != 0 {
		return nil, fmt.Errorf("invalid UDP peer list")
	}

	return parseCompactPeers(peerBytes)
}
