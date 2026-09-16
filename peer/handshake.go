package peer

/*
pstrlen - 1 byte
pstr - 19 bytes
reserved - 8 bytes
info_hash - 20 bytes
peer_id - 20 bytes
*/
import (
	"fmt"
	"io"
	"net"
)

const (
	protocolString = "BitTorrent protocol" // has 19 characters
	handshakeLen   = 68
)

func Handshake(
	conn net.Conn, // TCP connection to other peer
	infoHash [20]byte, // torrent's identifier
	peerID [20]byte, // client's unique identifier
) ([20]byte, error) {

	var result [20]byte

	request := make([]byte, handshakeLen)

	// Pstrlen (for "BitTorrent protocol"): 19 decimal = 0x13 Hex
	request[0] = byte(len(protocolString))

	// Pstr: "BitTorrent protocol"
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

	/// This will store peer's handshake
	response := make([]byte, handshakeLen)

	/*
		TCP does not guarantee that one Read() gives you all 68 bytes.

		For example, the peer might send:

		68 bytes

		but TCP could deliver:

		20 bytes
		then
		30 bytes
		then
		18 bytes

		io.ReadFull() keeps reading until it gets the requested 68 bytes or encounters an error.
	*/
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

	/// Contains torrent's info hash
	var receivedHash [20]byte

	copy(
		receivedHash[:],
		response[28:48],
	)

	/// Verify if we are talking to the same torrent
	if receivedHash != infoHash {
		return result, fmt.Errorf(
			"info hash mismatch",
		)
	}

	/// Extract Remote Peer ID and store it in result and return
	copy(
		result[:],
		response[48:68],
	)

	return result, nil
}