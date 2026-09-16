/*This represents a peer connection.*/

package peer

import (
	"fmt"
	"net"
	"time"
)

type Connection struct {
	Conn net.Conn // TCP connection to peer

	PeerID [20]byte // Identity of peer

	Choked bool // Can peer currently send us data

	Interested bool // Are we interested on downloading from peer

	Bitfield []byte // Which pieces does peer have

	LastActivity time.Time // When did we last communicate with peer later
}

func Connect(
	address string, // where is the peer?
	infoHash [20]byte, // which torrent?
	peerID [20]byte, // who are we?
) (*Connection, error) {

	conn, err := net.DialTimeout(
		"tcp",
		address,
		10*time.Second,
	)

	if err != nil {
		return nil, err
	}

	remotePeerID, err := Handshake(
		conn,
		infoHash,
		peerID,
	)

	if err != nil {
		conn.Close()
		return nil, err
	}

	c := &Connection{
		Conn:         conn,
		PeerID:       remotePeerID,
		Choked:       true, /// We assume peer is choking us until we receive UNCHOKE
		LastActivity: time.Now(),
	}

	if err := WriteInterested(conn); err != nil {
		conn.Close()
		return nil, err
	}

	return c, nil
}

func (c *Connection) Close() {
	if c.Conn != nil {
		c.Conn.Close()
	}
}

func (c *Connection) String() string {
	return fmt.Sprintf(
		"peer=%s",
		c.Conn.RemoteAddr().String(),
	)
}
