/*
4 bytes length
1 byte message ID
N bytes payload
*/

package peer

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	Choke         byte = 0
	Unchoke       byte = 1
	Interested    byte = 2
	NotInterested byte = 3
	Have          byte = 4
	Bitfield      byte = 5
	Request       byte = 6
	Piece         byte = 7
	Cancel        byte = 8
)

type Message struct {
	ID      byte   // what message is this?
	Payload []byte // Data belonging to that message
}

// This reads a BitTorrent message from the network.
func ReadMessage(r io.Reader) (*Message, error) {
	/*
		┌────────────┬──────────┬──────────────┐
		│ 4-byte     │ 1-byte   │ N-byte       │
		│ length     │ ID       │ payload      │
		└────────────┴──────────┴──────────────┘
					  ← length ───────────────→

	*/
	var length uint32

	/// The first 4 bytes of every BitTorrent message tell us how many bytes follow.
	/*
		The length includes:
		message ID + payload
	*/
	if err := binary.Read(
		r,
		binary.BigEndian,
		&length,
	); err != nil {
		return nil, err
	}

	/*
		A BitTorrent message with:
		length = 0
		is a keep-alive.

		So 00 00 00 00 means I'm still connected
	*/
	if length == 0 {
		return nil, nil
	}

	/*
			Prevent an enormous allocation
		    without this check, a malicious peer could send

			FF FF FF FF

			That could make your program try to allocate an enormous amount of memory. So this is a safety limit.


	*/
	if length > 1<<20 {
		return nil, fmt.Errorf(
			"message too large: %d",
			length,
		)
	}

	// Allocate the message data
	data := make([]byte, length)

	// ReadFull gets data even when data are received in chunks
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}

	return &Message{
		ID:      data[0],
		Payload: data[1:],
	}, nil
}

func WriteMessage(
	w io.Writer,
	id byte,
	payload []byte,
) error {
	/*
		┌────────────┬──────────┬──────────────┐
		│ 4-byte     │ 1-byte   │ N-byte       │
		│ length     │ ID       │ payload      │
		└────────────┴──────────┴──────────────┘
	*/
	// length = length of the message ID (i.e. 1) + length of the payload
	length := uint32(1 + len(payload))

	var header [4]byte

	/// Convert the length to 4 bytes
	/*
		Suppose:

		length = 13

		Then:

		13 decimal
		    ↓
		00 00 00 0D

	*/
	binary.BigEndian.PutUint32(
		header[:],
		length,
	)

	// Write the length
	if _, err := w.Write(header[:]); err != nil {
		return err
	}

	// Write the message ID
	if _, err := w.Write([]byte{id}); err != nil {
		return err
	}

	/*
		If there is payload, write it.
		If there isn't, skip it.

	*/
	if len(payload) > 0 {
		_, err := w.Write(payload)
		return err
	}

	return nil
}

func WriteInterested(w io.Writer) error {
	return WriteMessage(
		w,
		Interested,
		nil,
	)
}

func WriteRequest(
	w io.Writer,
	index uint32,
	begin uint32,
	length uint32,
) error {
	/*
		┌──────────────┬──────────────┬──────────────┐
		│ index        │ begin        │ length       │
		│ 4 bytes      │ 4 bytes      │ 4 bytes      │
		└──────────────┴──────────────┴──────────────┘
	*/

	payload := make([]byte, 12)

	binary.BigEndian.PutUint32(
		payload[0:4],
		index,
	)

	binary.BigEndian.PutUint32(
		payload[4:8],
		begin,
	)

	binary.BigEndian.PutUint32(
		payload[8:12],
		length,
	)

	return WriteMessage(
		w,
		Request,
		payload,
	)
}
