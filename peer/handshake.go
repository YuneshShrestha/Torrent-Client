package peer

/*
pstrlen
pstr
reserved
info_hash
peer_id
*/
import (
	"fmt"
	"io"
	"net"
)

const (
	protocolString = "BitTorrent protocol"
	handshakeLen   = 68
)

func Handshake(
	conn net.Conn,
	infoHash [20]byte,
	peerID [20]byte,
) ([20]byte, error) {

	var result [20]byte

	request := make([]byte, handshakeLen)

	request[0] = byte(len(protocolString))

	copy(
		request[1:20],
		protocolString,
	)

	// Reserved bytes: 20-27.
	// Leave as zero.

	copy(
		request[28:48],
		infoHash[:],
	)

	copy(
		request[48:68],
		peerID[:],
	)

	if _, err := conn.Write(request); err != nil {
		return result, err
	}

	response := make([]byte, handshakeLen)

	if _, err := io.ReadFull(
		conn,
		response,
	); err != nil {
		return result, err
	}

	if response[0] != byte(len(protocolString)) {
		return result, fmt.Errorf(
			"invalid protocol string length",
		)
	}

	if string(response[1:20]) != protocolString {
		return result, fmt.Errorf(
			"invalid BitTorrent protocol",
		)
	}

	var receivedHash [20]byte

	copy(
		receivedHash[:],
		response[28:48],
	)

	if receivedHash != infoHash {
		return result, fmt.Errorf(
			"info hash mismatch",
		)
	}

	copy(
		result[:],
		response[48:68],
	)

	return result, nil
}