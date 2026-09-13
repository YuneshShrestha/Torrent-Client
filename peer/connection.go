/*This represents a peer connection.*/

package peer

import (
	"fmt"
	"net"
	"time"
)

type Connection struct {
	Conn net.Conn

	PeerID [20]byte

	Choked bool

	Interested bool

	Bitfield []byte

	LastActivity time.Time
}

func Connect(
	address string,
	infoHash [20]byte,
	peerID [20]byte,
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
		Choked:       true,
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
